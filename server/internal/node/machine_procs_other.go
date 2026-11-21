//go:build !linux && !darwin

package node

import (
	"context"
	"errors"

	"github.com/nwverse/persona/server/internal/proto"
)

func osListListeningPorts(context.Context, string) ([]proto.ListeningPort, bool, string, error) {
	return nil, false, "listening port inspection is supported on Linux and macOS", nil
}

func osListProcesses(context.Context) ([]rawProcess, int64, bool, string, error) {
	return nil, 0, false, "process inspection is supported on Linux and macOS", nil
}

func osSendProcessSignal(int, string) error {
	return errors.New("process termination is supported on Linux and macOS")
}
