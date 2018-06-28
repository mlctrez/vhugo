package devicedb

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/mlctrez/vhugo/hlog"
	"github.com/mlctrez/vhugo/tmpl"
	"gopkg.in/satori/go.uuid.v1"
)

type DeviceDB struct {
	logger *hlog.HLog
	DB     *bolt.DB
}

func New(path string, logger *log.Logger) (db *DeviceDB, err error) {
	db = &DeviceDB{logger: hlog.New(logger, "DeviceDB")}
	options := &bolt.Options{Timeout: 5 * time.Second}
	db.DB, err = bolt.Open(path, 0600, options)
	return
}

func (d *DeviceDB) Close() error {
	d.logger.Println("Close()")
	return d.DB.Close()
}

type DeviceGroup struct {
	ServerIP   string
	ServerPort int
	GroupID    string
	UUID       string
	UU         string
}

func NewDeviceGroup(groupID string) *DeviceGroup {
	uuID := uuid.NewV4()
	parts := strings.Split(uuID.String(), "-")
	return &DeviceGroup{
		GroupID: groupID,
		UUID:    uuID.String(),
		UU:      parts[len(parts)-1],
	}
}

func (dg *DeviceGroup) Setup() (setupXml []byte, err error) {
	buf := &bytes.Buffer{}
	if err = tmpl.SettingsTemplate.Execute(buf, dg); err == nil {
		setupXml = buf.Bytes()
	}
	return
}

func (d *DeviceDB) deviceGroupsUpdate(fn func(dgBucket *bolt.Bucket) error) error {
	return d.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("deviceGroups"))
		if err != nil {
			return err
		}
		return fn(b)
	})
}

func (d *DeviceDB) GetDeviceGroups() (deviceGroups []*DeviceGroup, err error) {
	deviceGroups = make([]*DeviceGroup, 0)

	err = d.deviceGroupsUpdate(func(dgBucket *bolt.Bucket) error {
		c := dgBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			dg := &DeviceGroup{}
			if err = json.Unmarshal(v, dg); err == nil {
				deviceGroups = append(deviceGroups, dg)
			} else {
				if err := dgBucket.Delete(k); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return
}

func (d *DeviceDB) AddDeviceGroup(dg *DeviceGroup) error {
	return d.deviceGroupsUpdate(func(dgBucket *bolt.Bucket) error {
		if dgBucket.Get([]byte(dg.GroupID)) != nil {
			return fmt.Errorf("device group %s already exists", dg.GroupID)
		}
		if dgBytes, err := json.Marshal(dg); err != nil {
			return err
		} else {
			return dgBucket.Put([]byte(dg.GroupID), dgBytes)
		}
	})
}

func (d *DeviceDB) GetDeviceGroup(groupID string) (dg *DeviceGroup, err error) {
	err = d.deviceGroupsUpdate(func(dgBucket *bolt.Bucket) error {
		dgBytes := dgBucket.Get([]byte(groupID))
		if dgBytes == nil {
			return fmt.Errorf("device group %s does not exist", groupID)
		} else {
			dg = &DeviceGroup{}
			return json.Unmarshal(dgBytes, dg)
		}
	})
	return
}

func (d *DeviceDB) DeleteDeviceGroup(groupID string) error {
	return d.deviceGroupsUpdate(func(dgBucket *bolt.Bucket) error {
		if dgBucket.Get([]byte(groupID)) == nil {
			return fmt.Errorf("device group %s does not exist", groupID)
		} else {
			return dgBucket.Delete([]byte(groupID))
		}
	})
}

func NewVirtualLight(name string) *VirtualLight {
	return &VirtualLight{
		State: VirtualLightState{
			On:        false,
			Bri:       0,
			Hue:       13088,
			Sat:       212,
			Xy:        []float32{0.5128, 0.4147},
			Ct:        467,
			Alert:     "none",
			Effect:    "none",
			Colormode: "xy",
			Reachable: true,
		},
		Type:      "Extended color light",
		Name:      name,
		Modelid:   "LCT001",
		Swversion: "66009461",
		Pointsymbol: map[string]string{
			"1": "none",
			"2": "none",
			"3": "none",
			"4": "none",
			"5": "none",
			"6": "none",
			"7": "none",
			"8": "none",
		},
	}
}

type VirtualLight struct {
	State       VirtualLightState `json:"state"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Modelid     string            `json:"modelid"`
	Swversion   string            `json:"swversion"`
	Pointsymbol map[string]string `json:"pointsymbol"`
}

func (vl *VirtualLight) UpdateState(sr *StateRequest) {
	if sr.On != nil {
		vl.State.On = *sr.On
	}
	if sr.Bri != nil {
		vl.State.Bri = *sr.Bri
	}
}

func (d *DeviceDB) virtualLightsUpdate(groupID string, fn func(vlBucket *bolt.Bucket) error) error {
	return d.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(groupID + "_virtualLights"))
		if err != nil {
			return err
		}
		return fn(b)
	})
}

func (d *DeviceDB) GetVirtualLights(groupID string) (lights map[string]*VirtualLight, err error) {
	lights = make(map[string]*VirtualLight)

	err = d.virtualLightsUpdate(groupID, func(vlBucket *bolt.Bucket) error {
		c := vlBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			vl := &VirtualLight{}
			if err = json.Unmarshal(v, vl); err == nil {
				lights[string(k)] = vl
			} else {
				if err := vlBucket.Delete(k); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return
}

func (d *DeviceDB) GetVirtualLight(groupID string, lightID string) (virtualLight *VirtualLight, err error) {
	err = d.virtualLightsUpdate(groupID, func(vlBucket *bolt.Bucket) error {
		virtualLightBytes := vlBucket.Get([]byte(lightID))
		if virtualLightBytes == nil {
			return fmt.Errorf("virtual light %s does not exist in group %s", lightID, groupID)
		} else {
			virtualLight = &VirtualLight{}
			return json.Unmarshal(virtualLightBytes, virtualLight)
		}
	})
	return
}

func (d *DeviceDB) DeleteVirtualLight(groupID string, lightID string) error {
	return d.virtualLightsUpdate(groupID, func(vlBucket *bolt.Bucket) error {
		key := []byte(lightID)
		if vlBucket.Get(key) == nil {
			return fmt.Errorf("virtual light %s does not exist in group %s", lightID, groupID)
		} else {
			return vlBucket.Delete(key)
		}
	})
}

func (d *DeviceDB) UpdateVirtualLight(groupID string, virtualLight *VirtualLight) (err error) {
	return d.virtualLightsUpdate(groupID, func(vlBucket *bolt.Bucket) error {
		if vlBytes, errMarshal := json.Marshal(virtualLight); err != nil {
			return errMarshal
		} else {
			return vlBucket.Put([]byte(Sha(virtualLight.Name)), vlBytes)
		}
	})
}

func Sha(name string) string {
	hash := sha256.New()
	io.WriteString(hash, name)
	return fmt.Sprintf("%x", hash.Sum(nil))
}

type VirtualLightState struct {
	On        bool      `json:"on"`
	Bri       int32     `json:"bri"`
	Hue       int32     `json:"hue"`
	Sat       int32     `json:"sat"`
	Xy        []float32 `json:"xy"`
	Ct        int32     `json:"ct"`
	Alert     string    `json:"alert"`
	Effect    string    `json:"effect"`
	Colormode string    `json:"colormode"`
	Reachable bool      `json:"reachable"`
}

type StateRequest struct {
	On  *bool  `json:"on"`
	Bri *int32 `json:"bri"`
}
