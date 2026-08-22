package syver

import (
	"os"
	"reflect"
	"strings"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// AddResources is a simple wrapper to add multiple resources
func AddResources(fileName, resourceName string, keys []string, c *util.Config) error {
	var err error
	err = setLogLevel(c)
	if err != nil {
		return err
	}
	outStoreFormat, err = getStoreFormatFromFileName(fileName)
	if err != nil {
		return err
	}

	var syverConfig SyverConfig
	if _, err := os.Stat(fileName); err == nil {
		syverConfig, err = ReadJSON(fileName)
		if err != nil {
			return err
		}
	} else {
		syverConfig = *NewSyverConfig()
	}

	sys := system.New(c.PackageManager)

	for _, key := range keys {
		if err := AddResource(fileName, syverConfig, resourceName, key, *c, sys); err != nil {
			return err
		}
	}

	return WriteJSON(fileName, syverConfig)
}

// AddResource adds a single resource to fileName.
//
// FEAT-007 G1: this used to be a 16-case switch on resourceName, each case
// hand-calling syverConfig.<Field>.AppendSysResource(...) directly. That's
// replaced with a descriptor lookup by Name (desc.AppendSys, which closes
// over the type-specific system+resource construction -- see
// resource/descriptor.go) plus resource.UpsertLive, which needs to know
// *which* SyverConfig field to insert into without resource-package code
// ever importing SyverConfig (the two packages can't import each other).
// configAccessors[desc.Key].Field supplies that: a pointer to the live
// struct field, e.g. &syverConfig.Ports -- not a copy, so the new resource
// actually lands in the file WriteJSON writes below. Using the Get member
// here instead (a snapshot copy) was the exact trap this spec calls out:
// `syver add` would silently stop writing new resources.
func AddResource(fileName string, syverConfig SyverConfig, resourceName, key string, config util.Config, sys *system.System) error {
	desc, ok := resource.DescriptorByName(resourceName)
	if !ok || desc.AppendSys == nil {
		return unrecognizedResourceType(resourceName)
	}
	acc, ok := configAccessors[desc.Key]
	if !ok || acc.Field == nil {
		return unrecognizedResourceType(resourceName)
	}

	built, err := desc.AppendSys(sys, key, config)
	if err != nil {
		return err
	}
	if err := resource.UpsertLive(acc.Field(&syverConfig), built); err != nil {
		return err
	}

	res, _ := built.(resource.ResourceRead)
	resourcePrint(fileName, res, config.AnnounceToCLI)

	return nil
}

// AutoAddResources is a simple wrapper to add multiple resources
func AutoAddResources(fileName string, keys []string, c *util.Config) error {
	var err error
	outStoreFormat, err = getStoreFormatFromFileName(fileName)
	if err != nil {
		return err
	}

	var syverConfig SyverConfig
	if _, err = os.Stat(fileName); err == nil {
		syverConfig, err = ReadJSON(fileName)
		if err != nil {
			return err
		}
	} else {
		syverConfig = *NewSyverConfig()
	}

	sys := system.New(c.PackageManager)

	for _, key := range keys {
		if err := AutoAddResource(fileName, syverConfig, key, c, sys); err != nil {
			return err
		}
	}

	return WriteJSON(fileName, syverConfig)
}

// appendIfExistsGeneric reflectively calls field's AppendSysResourceIfExists
// method (field is what configAccessors[key].Field(&syverConfig) returns,
// e.g. *resource.PortMap) and reports the built resource plus whether it
// existed. Used by AutoAddResource's generic loop below for every AutoAdd
// type except the two with genuinely special-cased logic (file, process --
// see §4.6): AppendSysResourceIfExists's PT/ST return types vary per
// resource type, so there's no shared non-generic Go interface to call it
// through directly, the same reason add.go's own dispatch above needs
// UpsertLive instead of a common method signature.
func appendIfExistsGeneric(field any, key string, sys *system.System) (resource.ResourceRead, bool, error) {
	method := reflect.ValueOf(field).Elem().MethodByName("AppendSysResourceIfExists")
	results := method.Call([]reflect.Value{reflect.ValueOf(key), reflect.ValueOf(sys)})
	ok := results[2].Bool()
	if errI := results[3].Interface(); errI != nil {
		return nil, ok, errI.(error)
	}
	res, _ := results[0].Interface().(resource.ResourceRead)
	return res, ok, nil
}

// AutoAddResource adds a single resource to fileName with automatic detection of the type of resource
func AutoAddResource(fileName string, syverConfig SyverConfig, key string, c *util.Config, sys *system.System) error {
	for _, desc := range resource.AutoAddDescriptors() {
		switch desc.Key {
		case "file":
			// Special-cased per §4.6: only scanned when key looks like a
			// path, unlike every other AutoAdd type.
			if !strings.Contains(key, "/") {
				continue
			}
			res, _, ok, err := syverConfig.Files.AppendSysResourceIfExists(key, sys)
			if err != nil {
				return err
			}
			if ok {
				resourcePrint(fileName, res, c.AnnounceToCLI)
			}

		case "process":
			// Special-cased per §4.6: on a match, also fans out to every
			// listening port owned by that process's PIDs.
			res, sysres, ok, err := syverConfig.Processes.AppendSysResourceIfExists(key, sys)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			resourcePrint(fileName, res, c.AnnounceToCLI)
			ports, err := system.GetPorts()
			if err != nil {
				return err
			}
			pids, _ := sysres.Pids()
			for _, pid := range pids {
				for port, entries := range ports {
					for _, entry := range entries {
						if entry.Pid == int32(pid) {
							if res, _, ok, err := syverConfig.Ports.AppendSysResourceIfExists(port, sys); err != nil {
								return err
							} else if ok {
								resourcePrint(fileName, res, c.AnnounceToCLI)
							}
						}
					}
				}
			}

		default:
			field := configAccessors[desc.Key].Field(&syverConfig)
			res, ok, err := appendIfExistsGeneric(field, key, sys)
			if err != nil {
				return err
			}
			if ok {
				resourcePrint(fileName, res, c.AnnounceToCLI)
			}
		}
	}

	return nil
}
