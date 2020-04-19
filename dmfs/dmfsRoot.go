package dmfs

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/pkg/errors"
)

/*
* the root fs is supposed to load and interact
* with the namespaces and to show them as directories
 */

type rootNode struct {
	fs.Inode

	loading bool
	nsNodes map[string]*namespaceNode

	mx sync.Mutex
}

// Implement required interfaces
var (
	_ = (fs.NodeReaddirer)((*rootNode)(nil))
	_ = (fs.NodeRenamer)((*rootNode)(nil))
	_ = (fs.NodeRmdirer)((*rootNode)(nil))
	_ = (fs.NodeLookuper)((*rootNode)(nil))
	_ = (fs.NodeGetattrer)((*rootNode)(nil))
)

var (
	// ErrAlreadyLoading error if a load process is already running
	ErrAlreadyLoading = errors.New("already loading")
)

// Create new root Node
func newRootNode() *rootNode {
	rn := &rootNode{}
	rn.nsNodes = make(map[string]*namespaceNode)
	return rn
}

// On dir access, load namespaces and groups
func (root *rootNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	r := make([]fuse.DirEntry, 0)

	// Load namespaces and groups
	err := root.load(func(name string) {
		r = append(r, fuse.DirEntry{
			Name: name,
			Mode: syscall.S_IFDIR,
		})
	})

	// If already loading, use current cache
	if err != nil && err != ErrAlreadyLoading {
		fmt.Println(err)
		return nil, syscall.EIO
	}

	return fs.NewListDirStream(r), 0
}

// Load groups and namespaces
func (root *rootNode) load(nsCB func(name string)) error {
	if root.loading {
		return ErrAlreadyLoading
	}

	root.mx.Lock()
	root.loading = true

	defer func() {
		// Unlock and set loading to false
		// at the end
		root.loading = false
		root.mx.Unlock()
	}()

	// Use dataStore to retrieve
	// groups and namespaces
	err := data.loadUserAttributes()
	if err != nil {
		return err
	}

	// Loop Namespaces and add childs in as folders
	for _, namespace := range data.userAttributes.Namespace {
		nsName := removeNSName(namespace.Name)

		// Find namespace node
		v, has := root.nsNodes[nsName]
		if !has {
			// Create new if not exists
			root.nsNodes[nsName] = newNamespaceNode(namespace)
		} else {
			// Update groups if exists
			v.nsInfo.Groups = namespace.Groups
		}

		if nsCB != nil {
			nsCB(nsName)
		}
	}

	return nil
}

// Lookup -> something tries to lookup a file (namespace)
func (root *rootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// Get cached namespaceInfo from map
	val, has := root.nsNodes[name]
	if !has {
		return nil, syscall.ENOENT
	}

	// Try to reuse child
	child := root.GetChild(name)

	// Create new child if not found
	if child == nil {
		child = root.NewInode(ctx, val, fs.StableAttr{
			Mode: syscall.S_IFDIR,
		})
	}

	return child, 0
}

// Delete Namespace if virtual file was unlinked
func (root *rootNode) Rmdir(ctx context.Context, name string) syscall.Errno {
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

// Rename namespace if virtual file was renamed
func (root *rootNode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	// Don't rename default ns
	if name == "default" {
		fmt.Println("Can't rename default namespace!")
		return syscall.EIO
	}

	// Get real namespace names
	oldNSName := addNSName(name, data.libdm.Config)
	newNSName := addNSName(newName, data.libdm.Config)
	root.debug("rename namespace", oldNSName, "->", newNSName)

	// Make rename request
	_, err := data.libdm.UpdateNamespace(oldNSName, newNSName)
	if err != nil {
		fmt.Println(err)
		return syscall.EIO
	}

	// Return success
	return 0
}

// Set attributes for files
func (root *rootNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// Set owner/group
	data.setUserAttr(out)
	return 0
}

func (root *rootNode) debug(arg ...interface{}) {
	if data.mounter.Debug {
		fmt.Println(arg...)
	}
}
