package stage

// Stage represents execution stages.
type Stage int

const (
	// None is zero value of Stage
	None Stage = iota
	// Recv means "received message, but not executed"
	Recv
	// Lock means "waiting execution in queue"
	Lock
	// Exec means "executing"
	Exec
	// Done means "done execution"
	Done
)

// MarshalBinary marshal Stage into []byte.  used by go-redis/redis.
func (stg Stage) MarshalBinary() ([]byte, error) {
	return []byte{byte(stg)}, nil
}

func (stg Stage) String() string {
	switch stg {
	case None:
		return "None"
	case Recv:
		return "Recv"
	case Lock:
		return "Lock"
	case Exec:
		return "Exec"
	case Done:
		return "Done"
	default:
		return "Unknown"
	}
}
