//go:build !linux

package run

import (
	"context"
	"fmt"
	"os"
)

func createNative(context.Context, *Client, EnvironmentPolicy) (*box, *os.File, error) {
	return nil, nil, fmt.Errorf("sandbox environments require Linux")
}
