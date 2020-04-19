package dmfs

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
)

type dmanagerRoot struct {
	fs.Inode
}

// implement the interfaces
var _ = (fs.NodeOnAdder)((*dmanagerRoot)(nil))
var _ = (fs.NodeRenamer)((*dmanagerRoot)(nil))
var _ = (fs.NodeRmdirer)((*dmanagerRoot)(nil))

// OnAdd is called on mounting the file system. Use it to populate
// the file system tree.
func (root *dmanagerRoot) OnAdd(ctx context.Context) {
	root.debug("Init files")

	err := data.loadUserAttributes()
	if err != nil {
		log.Fatal(err)
		return
	}

	// Loop Namespaces and add childs in as folders
	for _, namespace := range data.userAttributes.Namespace {
		nsName := removeNSName(namespace.Name)

		// reuse child
		nsp := root.Inode.GetChild(nsName)

		// Create namespace folder
		if nsp == nil {
			nsp = root.Inode.NewInode(ctx, &fs.Inode{}, fs.StableAttr{
				Mode: syscall.S_IFDIR,
			})
			root.AddChild(nsName, nsp, true)
		}

		// Use a no_group folder for files
		// not associated to a groud
		if len(namespace.Groups) == 0 {
			namespace.Groups = []string{"no_group"}
		}

		// Add groups to namespace
		for _, group := range namespace.Groups {
			gp := nsp.GetChild(group)
			if gp == nil {
				gp = nsp.NewInode(ctx, &groupInode{
					group:     group,
					namespace: namespace.Name,
				}, fs.StableAttr{
					Mode: syscall.S_IFDIR,
				})

				nsp.AddChild(group, gp, true)
			}
		}
	}

	root.debug("Init files success")

}

// Unlink if virtual file was unlinked
func (root *dmanagerRoot) Rmdir(ctx context.Context, name string) syscall.Errno {
	namespace := addNSName(name, data.libdm.Config)

	// wait 2 seconds to ensure, user didn't cancel
	select {
	case <-ctx.Done():
		return syscall.ECANCELED
	case <-time.After(2 * time.Second):
	}

	// Do delete request
	if _, err := data.libdm.DeleteNamespace(namespace); err != nil {
		fmt.Println(err)
		return syscall.EFAULT
	}

	return 0
}

// Rename if virtual file was renamed
func (root *dmanagerRoot) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	// Don't rename default ns
	if name == "default" {
		fmt.Println("Can't rename default namespace!")
		return syscall.EACCES
	}

	// Get real namespace names
	oldNSName := addNSName(name, data.libdm.Config)
	newNSName := addNSName(newName, data.libdm.Config)
	root.debug("rename namespace", oldNSName, "->", newNSName)

	// Make rename request
	_, err := data.libdm.UpdateNamespace(oldNSName, newNSName)
	if err != nil {
		fmt.Println(err)
		return syscall.ENONET
	}

	// Return success
	return 0
}

func (root *dmanagerRoot) debug(arg ...interface{}) {
	if data.mounter.Debug {
		fmt.Println(arg...)
	}
}
