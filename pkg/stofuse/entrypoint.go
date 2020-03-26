// FUSE adapter for interfacing with Varasto from filesystem
package stofuse

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/function61/gokit/logex"
	"github.com/function61/gokit/ossignal"
	"github.com/function61/gokit/taskrunner"
	"github.com/function61/varasto/pkg/stoclient"
	"github.com/spf13/cobra"
)

func Entrypoint() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fuse",
		Short: "Varasto-FUSE integration",
	}

	addr := ":8689"
	unmountFirst := false

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Mounts a FUSE-based FS to serve collections from Varasto",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			rootLogger := logex.StandardLogger()

			ctx, cancel := context.WithCancel(ossignal.InterruptOrTerminateBackgroundCtx(
				rootLogger))
			defer cancel()

			go func() {
				// wait for stdin EOF (or otherwise broken pipe)
				_, _ = io.Copy(ioutil.Discard, os.Stdin)

				logex.Levels(rootLogger).Error.Println(
					"parent process died (detected by closed stdin) - stopping")

				cancel()
			}()

			if err := serve(ctx, addr, unmountFirst, rootLogger); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	serveCmd.Flags().StringVarP(&addr, "addr", "", addr, "Address to listen on")
	serveCmd.Flags().BoolVarP(&unmountFirst, "unmount-first", "u", unmountFirst, "Umount the mount-path first (maybe unclean shutdown previously)")

	cmd.AddCommand(serveCmd)

	return cmd
}

func serve(ctx context.Context, addr string, unmountFirst bool, logger *log.Logger) error {
	logl := logex.Levels(logger)

	conf, err := stoclient.ReadConfig()
	if err != nil {
		return err
	}

	// connects RPC API and FUSE server together
	sigs := newSigs()

	tasks := taskrunner.New(ctx, logger)

	tasks.Start("fusesrv", func(ctx context.Context) error {
		return fuseServe(ctx, sigs, *conf, unmountFirst, logl)
	})

	rpcStart(addr, sigs, tasks)

	return tasks.Wait()
}
