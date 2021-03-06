package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/fntlnz/mountinfo"
	"github.com/spf13/cobra"
)

type TraceRunnerOptions struct {
	podUID             string
	containerName      string
	inPod              bool
	programPath        string
	bpftraceBinaryPath string
}

func NewTraceRunnerOptions() *TraceRunnerOptions {
	return &TraceRunnerOptions{}
}

func NewTraceRunnerCommand() *cobra.Command {
	o := NewTraceRunnerOptions()
	cmd := &cobra.Command{
		PreRunE: func(c *cobra.Command, args []string) error {
			return o.Validate(c, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				fmt.Fprintln(os.Stdout, err.Error())
				return nil
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.containerName, "container", "c", o.containerName, "Specify the container")
	cmd.Flags().StringVarP(&o.podUID, "poduid", "p", o.podUID, "Specify the pod UID")
	cmd.Flags().StringVarP(&o.programPath, "program", "f", "program.bt", "Specify the bpftrace program path")
	cmd.Flags().StringVarP(&o.bpftraceBinaryPath, "bpftracebinary", "b", "/bin/bpftrace", "Specify the bpftrace binary path")
	cmd.Flags().BoolVar(&o.inPod, "inpod", false, "Wether or not run this bpftrace in a pod's container process namespace")
	return cmd
}

func (o *TraceRunnerOptions) Validate(cmd *cobra.Command, args []string) error {
	// TODO(fntlnz): do some more meaningful validation here, for now just checking if they are there
	if o.inPod == true && (len(o.containerName) == 0 || len(o.podUID) == 0) {
		return fmt.Errorf("poduid and container must be specified when inpod=true")
	}
	return nil
}

func (o *TraceRunnerOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *TraceRunnerOptions) Run() error {
	programPath := o.programPath
	if o.inPod == true {
		pid, err := findPidByPodContainer(o.podUID, o.containerName)
		if err != nil {
			return err
		}
		if pid == nil {
			return fmt.Errorf("pid not found")
		}
		if len(*pid) == 0 {
			return fmt.Errorf("invalid pid found")
		}
		f, err := ioutil.ReadFile(programPath)
		if err != nil {
			return err
		}
		programPath = path.Join(os.TempDir(), "program-container.bt")
		r := strings.Replace(string(f), "$container_pid", *pid, -1)
		if err := ioutil.WriteFile(programPath, []byte(r), 0755); err != nil {
			return err
		}
	}

	c := exec.Command(o.bpftraceBinaryPath, programPath)
	c.Stdout = os.Stdout
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr
	return c.Run()
}

func findPidByPodContainer(podUID, containerName string) (*string, error) {
	d, err := os.Open("/proc")

	if err != nil {
		return nil, err
	}

	defer d.Close()

	for {
		dirs, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, di := range dirs {
			if !di.IsDir() {
				continue
			}
			dname := di.Name()
			if dname[0] < '0' || dname[0] > '9' {
				continue
			}

			mi, err := mountinfo.GetMountInfo(path.Join("/proc", dname, "mountinfo"))
			if err != nil {
				continue
			}

			for _, m := range mi {
				root := m.Root
				if strings.Contains(root, podUID) && strings.Contains(root, containerName) {
					return &dname, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no process found for specified pod and container")
}
