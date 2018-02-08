package stage

type Stage int

const (
	None Stage = iota
	Recv
	Lock
	Exec
	Done
)

func (stg Stage) MarshalBinary() ([]byte, error) {
	return []byte{byte(stg)}, nil
}
