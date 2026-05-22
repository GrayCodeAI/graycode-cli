package netutil

const (
	LoopbackHost        = "127.0.0.1"
	LoopbackDynamicAddr = LoopbackHost + ":0"
	LoopbackNoProxy     = "localhost," + LoopbackHost
)
