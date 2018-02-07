package stage

type Stage int

const (
	None Stage = iota
	Recv
	Lock
	Exec
	Done
)
