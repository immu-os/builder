package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/go-debos/debos"
)

// NewFilesystemAction creates and manages a single filesystem image file, formats, mounts, and unmounts it.
type NewFilesystemAction struct {
	debos.BaseAction `yaml:",inline"`

	Path       string `yaml:"path"`       // Path to the image file to create
	Size       string `yaml:"size"`       // Size of the image file (e.g. 2G)
	Filesystem string `yaml:"filesystem"` // Filesystem type (ext4, xfs, vfat, ntfs, etc)
	Label      string `yaml:"label"`      // Optional label
	Mountpoint string `yaml:"mountpoint"` // Where to mount for subsequent actions
	Options    string `yaml:"options"`    // mkfs options

	mountTarget string // actual mount target used
}

func NewNewFilesystemAction() *NewFilesystemAction {
	return &NewFilesystemAction{
		BaseAction: debos.BaseAction{Action: "new-filesystem"},
	}
}

func (a *NewFilesystemAction) Verify(context *debos.DebosContext) error {
	if a.Path == "" || a.Size == "" || a.Filesystem == "" || a.Mountpoint == "" {
		return fmt.Errorf("new-filesystem: path, size, filesystem, and mountpoint are required")
	}
	return nil
}

func (a *NewFilesystemAction) Run(context *debos.DebosContext) error {
	imagePath := a.Path
	if !path.IsAbs(imagePath) {
		imagePath = path.Join(context.Artifactdir, imagePath)
	}
	// Create image file
	f, err := os.Create(imagePath)
	if err != nil {
		return fmt.Errorf("failed to create image file: %w", err)
	}
	defer f.Close()
	cmd := exec.Command("truncate", "-s", a.Size, imagePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set image size: %w", err)
	}
	// Format
	mkfs := "mkfs."
	mkfs += a.Filesystem
	mkfsArgs := []string{}
	if a.Label != "" {
		if strings.HasPrefix(a.Filesystem, "ext") || a.Filesystem == "xfs" || a.Filesystem == "vfat" || a.Filesystem == "ntfs" {
			mkfsArgs = append(mkfsArgs, "-L", a.Label)
		}
	}
	if a.Options != "" {
		mkfsArgs = append(mkfsArgs, strings.Fields(a.Options)...)
	}
	mkfsArgs = append(mkfsArgs, imagePath)
	if err := exec.Command(mkfs, mkfsArgs...).Run(); err != nil {
		return fmt.Errorf("failed to format image: %w", err)
	}
	// Mount
	mnt := a.Mountpoint
	if !path.IsAbs(mnt) {
		mnt = path.Join("/mnt", mnt)
	}
	if err := os.MkdirAll(mnt, 0755); err != nil {
		return fmt.Errorf("failed to create mountpoint: %w", err)
	}
	if err := exec.Command("mount", "-o", "loop", imagePath, mnt).Run(); err != nil {
		return fmt.Errorf("failed to mount image: %w", err)
	}
	a.mountTarget = mnt
	return nil
}

func (a *NewFilesystemAction) Cleanup(context *debos.DebosContext) error {
	if a.mountTarget != "" {
		exec.Command("umount", a.mountTarget).Run()
	}
	return a.BaseAction.Cleanup(context)
}

func (a *NewFilesystemAction) String() string {
	if a.Description == "" {
		return fmt.Sprintf("new-filesystem (image: %s, fs: %s, mount: %s)", a.Path, a.Filesystem, a.Mountpoint)
	}
	return a.Description
}
