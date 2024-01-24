package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stargate/stargate-grpc-go-client/stargate/pkg/auth"
	"github.com/stargate/stargate-grpc-go-client/stargate/pkg/client"
	pb "github.com/stargate/stargate-grpc-go-client/stargate/pkg/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func TestStargate(t *testing.T) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Failed to create docker pool: %+v", err)
	}

	// start cassandra
	dbOpts := &dockertest.RunOptions{
		Hostname:   `backend-1`,
		Repository: `cassandra`,
		Tag:        `3.11.13`,
		Env: []string{
			`HEAP_NEWSIZE=128M`,
			`MAX_HEAP_SIZE=1024M`,
			`CASSANDRA_SEEDS=backend-1`,
			`CASSANDRA_CLUSTER_NAME=c3-cluster`,
		},
		ExposedPorts: []string{`9042/tcp`},
	}
	dbRes, err := pool.RunWithOptions(dbOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	if err != nil {
		log.Fatalf("Failed to create cassandra: %+v", err)
	}
	defer dbRes.Close()

	// ensure already started, since cassandra might listen early even when it's not ready
	isReady := false
	go pool.Client.Logs(docker.LogsOptions{
		Context: context.Background(),

		Stderr:     true,
		Stdout:     true,
		Follow:     true,
		Timestamps: true,
		//RawTerminal: true,

		Container: dbRes.Container.ID,

		OutputStream: &tailer{
			OnWrite: func(s string) {
				if strings.Contains(s, `Created default superuser role`) {
					isReady = true
					fmt.Println(`cassandra ready!`)
				}
				fmt.Println(s)
			},
		},
	})

	// check whether ready
	if err := pool.Retry(func() error {
		if !isReady {
			return fmt.Errorf(`not ready`)
		}
		retURL := fmt.Sprintf("localhost:%s", dbRes.GetPort("9042/tcp"))
		port, _ := strconv.Atoi(dbRes.GetPort("9042/tcp"))
		clusterConfig := gocql.NewCluster(retURL)
		clusterConfig.Port = port
		log.Printf("%v", clusterConfig.Port)

		session, err := clusterConfig.CreateSession()
		if err != nil {
			return fmt.Errorf("error creating session: %s", err)
		}
		defer session.Close()
		return nil
	}); err != nil {
		log.Fatalf("Failed to connect to cassandra: %+v", err)
	}

	/* not needed?
	   cap_add:
	     - IPC_LOCK
	   ulimits:
	     memlock: -1
	*/

	// start stargate
	sgOpts := &dockertest.RunOptions{
		Hostname:     `stargate`,
		Repository:   `stargateio/stargate-3_11`,
		Tag:          `v1.0.77`,
		ExposedPorts: []string{`8090/tcp`, `8081/tcp`},
		Env: []string{
			`JAVA_OPTS=-Xmx2G`,
			`CLUSTER_NAME=c3-cluster`,
			`CLUSTER_VERSION=3.11`,
			`SEED=` + dbRes.Container.NetworkSettings.IPAddress,
			`RACK_NAME=rack1`,
			`DATACENTER_NAME=datacenter1`,
			`ENABLE_AUTH=true`,
		},
	}
	sgRes, err := pool.RunWithOptions(sgOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})

	// try listen log
	isReady = false
	go pool.Client.Logs(docker.LogsOptions{
		Context: context.Background(),

		Stderr:     true,
		Stdout:     true,
		Follow:     true,
		Timestamps: true,
		//RawTerminal: true,

		Container: sgRes.Container.ID,

		OutputStream: &tailer{
			OnWrite: func(s string) {
				if strings.Contains(s, `Finished starting bundles.`) {
					isReady = true
					fmt.Println(`stargate ready!`)
				}
				fmt.Println(s)
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create stargate: %+v", err)
	}
	defer sgRes.Close()

	//os.Setenv(`GRPC_TRACE`, `all`)
	//os.Setenv(`GRPC_VERBOSITY`, `DEBUG`)

	// wait stargate to be ready
	var stargateClient *client.StargateClient
	err = pool.Retry(func() error {
		if !isReady {
			return fmt.Errorf(`not ready`)
		}
		hostPort := sgRes.GetHostPort("8090/tcp")
		fmt.Println(hostPort)
		authHost := `http://` + sgRes.GetHostPort(`8081/tcp`) + `/v1/auth`
		fmt.Println(authHost)
		conn, err := grpc.Dial(hostPort,
			grpc.WithBlock(),
			grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.Config{BaseDelay: 1.0 * time.Second,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   20 * time.Second}}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(
				auth.NewTableBasedTokenProviderUnsafe(authHost, `cassandra`, `cassandra`),
			))
		if err != nil {
			fmt.Println(`grpc.Dial`, err)
			return err
		}
		connState := conn.GetState()
		if connState != connectivity.Ready {
			return fmt.Errorf(`not ready: %s`, connState)
		}
		stargateClient, err = client.NewStargateClientWithConn(conn)
		if err != nil {
			fmt.Printf("error NewStargateClientWithConn %v", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to connect to stargate: %+v", err)
	}

	// try run example query
	resp, err := stargateClient.ExecuteQuery(&pb.Query{
		Cql:        "SELECT * FROM system.local",
		Values:     nil,
		Parameters: nil,
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	result := resp.GetResultSet()

	for _, row := range result.Rows {
		for _, cell := range row.Values {
			fmt.Printf("%s ", cell)
		}
		fmt.Println()
	}
}

type tailer struct {
	OnWrite func(string)
}

func (t *tailer) Write(p []byte) (n int, err error) {
	if t.OnWrite != nil {
		t.OnWrite(string(p))
	}
	return len(p), nil
}
