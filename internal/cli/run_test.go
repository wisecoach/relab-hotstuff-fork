package cli

import (
	"testing"
)

func Test_Run(t *testing.T) {
	rootCmd.SetArgs([]string{"run", "--log-level", "debug"})
	rootCmd.Execute()
}
