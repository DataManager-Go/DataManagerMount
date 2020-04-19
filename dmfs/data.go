package dmfs

import (
	"os"
	"time"

	"github.com/DataManager-Go/libdatamanager"
	libdm "github.com/DataManager-Go/libdatamanager"
	dmConfig "github.com/DataManager-Go/libdatamanager/config"
)

var data dataStruct

// data provides the data for the fs
type dataStruct struct {
	mounter *Mounter
	config  *dmConfig.Config
	libdm   *libdatamanager.LibDM

	userAttributes   *libdm.UserAttributeDataResponse
	lastUserAttrLoad int64

	gid, uid uint32
}

func initData() {
	data.gid = uint32(os.Getegid())
	data.uid = uint32(os.Getuid())
}

// load user attributes (namespaces, groups)
func (data *dataStruct) loadUserAttributes() error {
	// TODO make cachhe time configureable or even create
	// a genuius way to calculate a reasonable cachetime

	if time.Now().Unix()-10 > data.lastUserAttrLoad {
		var err error
		data.userAttributes, err = data.libdm.GetUserAttributeData()
		if err != nil {
			return err
		}

		data.lastUserAttrLoad = time.Now().Unix()
	}

	return nil
}