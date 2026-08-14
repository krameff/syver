package syver

import (
	"fmt"
	"os"
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

// AddResource adds a single resource to fileName
func AddResource(fileName string, syverConfig SyverConfig, resourceName, key string, config util.Config, sys *system.System) error {
	var err error
	var res resource.ResourceRead

	// Need to figure out a good way to refactor this
	switch resourceName {
	case resource.AddResourceName:
		res, err = syverConfig.Addrs.AppendSysResource(key, sys, config)
	case resource.CommandResourceName:
		res, err = syverConfig.Commands.AppendSysResource(key, sys, config)
	case resource.DNSResourceName:
		res, err = syverConfig.DNS.AppendSysResource(key, sys, config)
	case resource.FileResourceName:
		res, err = syverConfig.Files.AppendSysResource(key, sys, config)
	case resource.GroupResourceName:
		res, err = syverConfig.Groups.AppendSysResource(key, sys, config)
	case resource.PackageResourceName:
		res, err = syverConfig.Packages.AppendSysResource(key, sys, config)
	case resource.PortResourceName:
		res, err = syverConfig.Ports.AppendSysResource(key, sys, config)
	case resource.ProcessResourceName:
		res, err = syverConfig.Processes.AppendSysResource(key, sys, config)
	case resource.ServiceResourceName:
		res, err = syverConfig.Services.AppendSysResource(key, sys, config)
	case resource.UserResourceName:
		res, err = syverConfig.Users.AppendSysResource(key, sys, config)
	case resource.SyverFileResourceName:
		res, err = syverConfig.Syverfiles.AppendSysResource(key, sys, config)
	case resource.KernelParamResourceName:
		res, err = syverConfig.KernelParams.AppendSysResource(key, sys, config)
	case resource.MountResourceName:
		res, err = syverConfig.Mounts.AppendSysResource(key, sys, config)
	case resource.InterfaceResourceName:
		res, err = syverConfig.Interfaces.AppendSysResource(key, sys, config)
	case resource.HTTPResourceName:
		res, err = syverConfig.HTTPs.AppendSysResource(key, sys, config)
	case resource.RegistryResourceName:
		res, err = syverConfig.Registries.AppendSysResource(key, sys, config)
	default:
		err = fmt.Errorf("undefined resource name: %s", resourceName)
	}

	if err != nil {
		return err
	}

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

// AutoAddResource adds a single resource to fileName with automatic detection of the type of resource
func AutoAddResource(fileName string, syverConfig SyverConfig, key string, c *util.Config, sys *system.System) error {
	// file
	if strings.Contains(key, "/") {
		res, _, ok, err := syverConfig.Files.AppendSysResourceIfExists(key, sys)
		if err != nil {
			return err
		}
		if ok {
			resourcePrint(fileName, res, c.AnnounceToCLI)
		}
	}

	// group
	if res, _, ok, err := syverConfig.Groups.AppendSysResourceIfExists(key, sys); err != nil {
		return err

	} else if ok {
		resourcePrint(fileName, res, c.AnnounceToCLI)
	}

	// package
	if res, _, ok, err := syverConfig.Packages.AppendSysResourceIfExists(key, sys); err != nil {

		return err

	} else if ok {
		resourcePrint(fileName, res, c.AnnounceToCLI)
	}

	// port
	if res, _, ok, err := syverConfig.Ports.AppendSysResourceIfExists(key, sys); err != nil {
		return err

	} else if ok {
		resourcePrint(fileName, res, c.AnnounceToCLI)
	}

	// process
	if res, sysres, ok, err := syverConfig.Processes.AppendSysResourceIfExists(key, sys); err != nil {
		return err
	} else if ok {
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
						// port
						if res, _, ok, err := syverConfig.Ports.AppendSysResourceIfExists(port, sys); err != nil {
							return err
						} else if ok {
							resourcePrint(fileName, res, c.AnnounceToCLI)
						}
					}
				}
			}
		}
	}

	// Service
	if res, _, ok, err := syverConfig.Services.AppendSysResourceIfExists(key, sys); err != nil {
		return err
	} else if ok {
		resourcePrint(fileName, res, c.AnnounceToCLI)
	}

	// user
	if res, _, ok, err := syverConfig.Users.AppendSysResourceIfExists(key, sys); err != nil {
		return err
	} else if ok {
		resourcePrint(fileName, res, c.AnnounceToCLI)
	}

	return nil
}
