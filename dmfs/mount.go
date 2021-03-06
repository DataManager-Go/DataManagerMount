package dmfs

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/DataManager-Go/libdatamanager"
	dmConfig "github.com/DataManager-Go/libdatamanager/config"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Mounter options to mount
type Mounter struct {
	MountPoint string
	Config     *dmConfig.Config
	Libdm      *libdatamanager.LibDM
	Debug      bool
	DebugFS    bool

	server   *fuse.Server
	doneChan chan bool
}

// Mount the fs
func (mounter *Mounter) Mount() {
	mountDir := filepath.Clean(mounter.MountPoint)
	fmt.Printf("Mounting on %s\n", mountDir)

	// Create mount dir if not exists
	if err := createMountpoint(mountDir); err != nil {
		log.Fatal(err)
	}

	rand.Seed(time.Now().Unix())

	// Test server availability
	if !mounter.testServer() {
		return
	}

	// Init exit channels
	exitChan := make(chan bool, 1)
	mounter.doneChan = make(chan bool, 1)

	data = dataStruct{
		mounter: mounter,
		config:  mounter.Config,
		libdm:   mounter.Libdm,
	}

	initData()

	// Create the fs
	root := newRootNode()

	var err error

	// Mount fs
	mounter.server, err = fs.Mount(mountDir, root, mounter.getMountOptions())
	if err != nil {
		log.Fatal(err)
	}

	// Umount fs on interrupt
	sigChan := make(chan os.Signal, 1)
	go (func() {
		signal.Notify(sigChan, os.Interrupt, os.Kill)

		// Await signal
		sig := <-sigChan

		// Debug & Umount
		fmt.Println("\rReceived", sig) // Print \r to overwrite the ugly ^C

		exitChan <- true
		mounter.umount()
	})()

	// Exit if mountpoint was
	// unmounted or process was interrupted
	mounter.server.Wait()
	select {
	case <-exitChan:
		<-mounter.doneChan
	default:
		fmt.Println("umounted externally\nexiting")
	}
}

// Umount fs
func (mounter *Mounter) umount() {
	// On error retry after 3s
	for {
		err := mounter.server.Unmount()

		if err != nil {
			fmt.Println(err)
			fmt.Println("retry in 3s")
			time.Sleep(3 * time.Second)
		} else {
			fmt.Println("Umounted")
			break
		}
	}

	mounter.doneChan <- true
}

// tests if server can be accessed and user is authorized
func (mounter *Mounter) testServer() bool {
	_, err := mounter.Libdm.GetNamespaces()
	if err != nil {
		fmt.Println("Can't mount:", err)
		return false
	}

	return true
}

// Get the mountoptions for the mount operation
func (mounter *Mounter) getMountOptions() *fs.Options {
	sec := time.Second

	return &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug:      mounter.DebugFS,
			AllowOther: false,
			FsName:     "Datamanager mount",
			Name:       "dmanager",
		},
		EntryTimeout: &sec,
		AttrTimeout:  &sec,
	}
}

// create dir if not exsists
func createMountpoint(mountPoint string) error {
	_, err := os.Stat(mountPoint)
	if err != nil {
		return os.Mkdir(mountPoint, 0700)
	}

	return nil
}
