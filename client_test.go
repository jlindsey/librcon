package rcon

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

const (
	addr = "127.0.0.1:25575"
	pw   = "rcon_test"
)

func TestLogin(t *testing.T) {
	Log.SetLevel(hclog.Trace)
	assert := assert.New(t)

	client, err := NewClient(addr, pw)
	if assert.NoError(err) {
		if assert.NoError(client.login()) {
			client.pw = "bad_pass"
			assert.Error(client.login())
		}
	}
}
