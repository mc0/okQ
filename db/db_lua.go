package db

import (
	"fmt"
	"strings"

	"github.com/mediocregopher/radix.v2/redis"
)

type lua struct {
	hash, script string
}

var luaScripts = map[string]*lua{
	"LREMRPUSH": {script: `local n = redis.call("LREM", KEYS[1], 0, ARGV[1])
		if n > 0 then
		    redis.call("RPUSH", KEYS[2], ARGV[1])
			end
		return n`,
	},
}

func initLuaScripts() error {
	for cmd, l := range luaScripts {
		hash, err := Cmd("SCRIPT", "LOAD", l.script).Str()
		if err != nil {
			return fmt.Errorf("loading %s: %s", cmd, err)
		}
		l.hash = hash
	}

	return nil
}

func luaHelper(
	c *redis.Client, cmd string, numKeys int, args ...interface{},
) *redis.Resp {

	cmd = strings.ToUpper(cmd)
	l, ok := luaScripts[cmd]
	if !ok {
		return redis.NewResp(fmt.Errorf("unknown lua script: %s", cmd))
	}

	realArgs := make([]interface{}, 0, len(args)+2)
	realArgs = append(realArgs, l.hash, numKeys)
	realArgs = append(realArgs, args...)

	r, notLoaded := luaEvalSha(c, realArgs)
	if !notLoaded {
		return r
	}

	if err := c.Cmd("SCRIPT", "LOAD", l.script).Err; err != nil {
		return r
	}

	r, _ = luaEvalSha(c, realArgs)
	return r
}

// Performs and EVALSHA with the given args, returning the reply and whether or
// not that reply is due to the script for that sha not being loaded yet
func luaEvalSha(c *redis.Client, args []interface{}) (*redis.Resp, bool) {
	r := c.Cmd("EVALSHA", args...)
	if r.Err != nil {
		if r.IsType(redis.AppErr) {
			return r, strings.HasPrefix(r.Err.Error(), "NOSCRIPT")
		}
	}
	return r, false
}
