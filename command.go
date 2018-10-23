package rcon

// CommandFuture is a deferred command result
type CommandFuture struct {
	ID      int32
	Command string
	Return  <-chan string
	Error   <-chan error
}

// Wait waits for the future to complete
func (cf *CommandFuture) Wait() (string, error) {
	select {
	case out := <-cf.Return:
		return out, nil
	case err := <-cf.Error:
		return "", err
	}
}
