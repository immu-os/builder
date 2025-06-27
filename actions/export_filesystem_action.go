package actions

import (
	"fmt"
	"log"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/go-debos/debos"
)

type ExportFilesystemAction struct {
	debos.BaseAction `yaml:",inline"`

	Source      string `yaml:"source"`
	Name        string `yaml:"name"`
	Destination string `yaml:"destination"`
	Trim        bool   `yaml:"trim"`
}

func NewExportFilesystemAction() *ExportFilesystemAction {
	return &ExportFilesystemAction{
		BaseAction: debos.BaseAction{Action: "export-filesystem"},
		Trim:       true,
	}
}

func (a *ExportFilesystemAction) Verify(context *debos.DebosContext) error {
	if a.Source == "" && a.Name == "" {
		return fmt.Errorf("export-filesystem: either source or name must be specified")
	}
	if a.Destination == "" {
		return fmt.Errorf("export-filesystem: destination must be specified")
	}
	return nil
}

func (a *ExportFilesystemAction) Run(context *debos.DebosContext) error {
	sourcePath := a.Source
	if sourcePath == "" && a.Name != "" {
		// Try /tmp/<name>.img, then /tmp/<name>, then any /tmp/<name>*
		candidates := []string{
			"/tmp/" + a.Name + ".img",
			"/tmp/" + a.Name,
		}
		found := false
		for _, candidate := range candidates {
			if _, err := exec.Command("test", "-e", candidate).Output(); err == nil {
				sourcePath = candidate
				found = true
				break
			}
		}
		if !found {
			// Fallback: glob for /tmp/<name>*
			globCmd := exec.Command("bash", "-c", "ls /tmp/"+a.Name+"*")
			out, err := globCmd.Output()
			if err == nil {
				files := strings.Fields(string(out))
				if len(files) > 0 {
					sourcePath = files[0]
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("export-filesystem: could not find a file for name '%s'", a.Name)
		}
	}
	if !path.IsAbs(sourcePath) {
		sourcePath = path.Join(context.Artifactdir, sourcePath)
	}
	destPath := a.Destination
	if !path.IsAbs(destPath) {
		destPath = path.Join(context.Artifactdir, destPath)
	}

	if a.Trim {
		if isImageMounted(sourcePath) {
			log.Printf("Image %s appears to be mounted, attempting to unmount", sourcePath)
			err := unmountImage(sourcePath)
			if err != nil {
				log.Printf("Warning: failed to unmount %s: %v", sourcePath, err)
			} else {
				log.Printf("Successfully unmounted %s", sourcePath)
			}
		}
		// Only try to trim ext2/3/4, vfat, ntfs, xfs, btrfs
		fsType, _ := runBlkidType(sourcePath)
		fsType = strings.TrimSpace(fsType)
		if strings.HasPrefix(fsType, "ext") {
			log.Printf("Trimming ext* filesystem: running e2fsck -f %s", sourcePath)
			e2fsckCmd := []string{"e2fsck", "-f", "-y", sourcePath}
			_ = exec.Command(e2fsckCmd[0], e2fsckCmd[1:]...).Run()
			log.Printf("Shrinking ext* filesystem: running resize2fs -M %s", sourcePath)
			resizeCmd := []string{"resize2fs", "-M", sourcePath}
			_ = exec.Command(resizeCmd[0], resizeCmd[1:]...).Run()
		}
		// Add more fs types as needed
	}
	// Copy image
	log.Printf("Exporting filesystem image %s to %s", sourcePath, destPath)
	blockSize := int64(4 * 1024 * 1024)
	cmd := exec.Command("dd", "if="+sourcePath, "of="+destPath, "bs="+strconv.FormatInt(blockSize, 10))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to export filesystem: %w", err)
	}
	log.Printf("Successfully exported filesystem %s to %s", sourcePath, destPath)
	return nil
}

func (a *ExportFilesystemAction) String() string {
	if a.Description == "" {
		return fmt.Sprintf("export-filesystem (source: %s, destination: %s)", a.Source, a.Destination)
	}
	return a.Description
}

// Helper: check if image file is mounted (by loop device)
func isImageMounted(imagePath string) bool {
	cmd := exec.Command("losetup", "-j", imagePath)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), imagePath)
}

// Helper: unmount image file by finding its mountpoint and loop device
func unmountImage(imagePath string) error {
	cmd := exec.Command("losetup", "-j", imagePath)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return fmt.Errorf("no loop device found for %s", imagePath)
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		loopdev := strings.Split(fields[0], ":")[0]
		mntCmd := exec.Command("findmnt", "-n", "-o", "TARGET", loopdev)
		mntOut, _ := mntCmd.Output()
		mnt := strings.TrimSpace(string(mntOut))
		if mnt != "" {
			exec.Command("umount", mnt).Run()
		}
		exec.Command("losetup", "-d", loopdev).Run()
	}
	return nil
}

// Helper to get filesystem type using blkid
func runBlkidType(device string) (string, error) {
	cmd := exec.Command("blkid", "-s", "TYPE", "-o", "value", device)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
