package main

type tailer struct {
	OnWrite func(string)
}

func (t *tailer) Write(p []byte) (n int, err error) {
	if t.OnWrite != nil {
		t.OnWrite(string(p))
	}
	return len(p), nil
}
