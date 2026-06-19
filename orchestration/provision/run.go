package provision

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

func logf(format string, args ...any) {
	log.Printf("[provision] "+format, args...)
}

// runEphemeral creates, starts, streams output from, waits on, and removes
// a container — the SDK equivalent of `docker run --rm`.
func runEphemeral(ctx context.Context, cli *client.Client, cfg *container.Config, host *container.HostConfig, name string) error {
	cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	resp, err := cli.ContainerCreate(ctx, cfg, host, nil, nil, name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	defer cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})

	waitCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}

	logs, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err == nil {
		stdcopy.StdCopy(os.Stdout, os.Stderr, logs)
		logs.Close()
	}

	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			return fmt.Errorf("%s exited %d", name, result.StatusCode)
		}
		return nil
	case err := <-errCh:
		return fmt.Errorf("wait %s: %w", name, err)
	}
}
