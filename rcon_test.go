package rcon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	addr = "127.0.0.1:25575"
	pw   = "rcon_test"
)

func TestLogin(t *testing.T) {
	assert.NoError(t, Login(addr, pw))
	assert.Error(t, Login(addr, "bad_pass"))
}
