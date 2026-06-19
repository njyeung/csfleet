package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

func ensureImage(ctx context.Context, cli *client.Client) error {
	logf("pulling %s", cs2Image)

	reader, err := cli.ImagePull(ctx, cs2Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", cs2Image, err)
	}
	defer reader.Close()

	dec := json.NewDecoder(reader)
	for {
		var msg struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read pull output: %w", err)
		}
		if msg.Error != "" {
			return fmt.Errorf("pull: %s", msg.Error)
		}
		if msg.Status != "" {
			fmt.Fprintln(os.Stdout, msg.Status)
		}
	}
	return nil
}
