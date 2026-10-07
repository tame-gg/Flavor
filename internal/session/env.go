package session

import (
	"os"
	"strconv"

	"tailscale.com/envknob"
)

var ambientAuthKeyVars = []string{"TS_AUTHKEY", "TS_AUTH_KEY"}

func PrepareProcessEnv() error {
	for _, k := range ambientAuthKeyVars {
		if err := os.Unsetenv(k); err != nil {
			return err
		}
	}
	envknob.SetNoLogsNoSupport()
	return nil
}

func processEnvPrepared() bool {
	for _, k := range ambientAuthKeyVars {
		if _, ok := os.LookupEnv(k); ok {
			return false
		}
	}
	noLogs, err := strconv.ParseBool(os.Getenv("TS_NO_LOGS_NO_SUPPORT"))
	return err == nil && noLogs
}
