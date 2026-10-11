package prism

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"ahv-console/internal/fault"
)

const imagesPath = "/api/vmm/v4.3/content/images"

type Drive struct {
	ID      string `json:"extId"`
	Address struct {
		Bus   string `json:"busType"`
		Index int    `json:"index"`
	} `json:"diskAddress"`
	Backing *struct {
		DiskID string `json:"diskExtId"`
		Source *struct {
			Reference struct {
				Type    string `json:"$objectType"`
				ImageID string `json:"imageExtId"`
			} `json:"reference"`
		} `json:"dataSource"`
	} `json:"backingInfo,omitempty"`
}

type Image struct {
	ID           string   `json:"extId"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Size         int64    `json:"sizeBytes"`
	Locations    []string `json:"clusterLocationExtIds"`
	Availability string   `json:"availability"`
}

func (image *Image) placement(cluster string) {
	image.Availability = "unknown"
	if cluster == "" || image.Locations == nil {
		return
	}
	for _, id := range image.Locations {
		if !uuidPattern.MatchString(id) {
			return
		}
	}
	image.Availability = "unavailable"
	for _, id := range image.Locations {
		if strings.EqualFold(id, cluster) {
			image.Availability = "ready"
		}
	}
}

// No partial result escapes on pagination or permission failure.
func collection[T any](ctx context.Context, c *Client, path, operation string, q url.Values, identifier func(T) string) ([]T, error) {
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	if q == nil {
		q = url.Values{}
	}
	result := make([]T, 0)
	seen := map[string]bool{}
	var total *int
	for page := 0; page < 10000; page++ {
		q.Set("$page", fmt.Sprint(page))
		q.Set("$limit", "100")
		var r struct {
			Data     []T `json:"data"`
			Metadata struct {
				Total *int `json:"totalAvailableResults"`
			} `json:"metadata"`
		}
		_, err := c.requestOperation(ctx, "GET", path, q, "", nil, operation, &r)
		if err != nil {
			return nil, err
		}
		if r.Data == nil && (r.Metadata.Total == nil || *r.Metadata.Total != 0) {
			return nil, fault.New(fault.Protocol, operation+" is missing its data array.")
		}
		if r.Metadata.Total != nil {
			if *r.Metadata.Total < 0 || (total != nil && *total != *r.Metadata.Total) {
				return nil, fault.New(fault.Protocol, operation+" changed during pagination; refresh inventory.")
			}
			total = r.Metadata.Total
		}
		for _, item := range r.Data {
			id := strings.ToLower(identifier(item))
			if !uuidPattern.MatchString(id) || seen[id] {
				return nil, fault.New(fault.Protocol, operation+" contains an invalid or repeated identifier; refresh inventory.")
			}
			seen[id] = true
			result = append(result, item)
		}
		if total != nil {
			if len(result) > *total || (len(r.Data) == 0 && len(result) < *total) {
				return nil, fault.New(fault.Protocol, operation+" returned an incomplete or inconsistent inventory.")
			}
			if len(result) == *total {
				return result, nil
			}
		} else if len(r.Data) == 0 {
			return result, nil
		}
	}
	return nil, fault.New(fault.Protocol, operation+" exceeded its pagination limit.")
}

func (c *Client) Drives(ctx context.Context, vmID string) ([]Drive, error) {
	if !uuidPattern.MatchString(vmID) {
		return nil, fault.New(fault.Configuration, "VM identifier must be a UUID.")
	}
	drives, err := collection(ctx, c, vmsPath+"/"+vmID+"/cd-roms", "CD-ROM inventory", nil, func(d Drive) string { return d.ID })
	if err != nil {
		return nil, err
	}
	sort.Slice(drives, func(i, j int) bool {
		a, b := drives[i], drives[j]
		if a.Address.Bus != b.Address.Bus {
			return a.Address.Bus < b.Address.Bus
		}
		if a.Address.Index != b.Address.Index {
			return a.Address.Index < b.Address.Index
		}
		return a.ID < b.ID
	})
	return drives, nil
}

func (c *Client) Image(ctx context.Context, id, cluster string) (Image, error) {
	if !uuidPattern.MatchString(id) {
		return Image{}, fault.New(fault.Configuration, "Image identifier must be a UUID.")
	}
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	var r struct {
		Data Image `json:"data"`
	}
	_, err := c.requestOperation(ctx, "GET", imagesPath+"/"+id, nil, "", nil, "ISO detail read", &r)
	if err != nil {
		return Image{}, err
	}
	if !strings.EqualFold(r.Data.ID, id) || r.Data.Type != "ISO_IMAGE" {
		return Image{}, fault.New(fault.Protocol, "Selected image is missing or no longer an ISO; refresh inventory.")
	}
	r.Data.placement(cluster)
	return r.Data, nil
}

func (c *Client) Images(ctx context.Context, cluster string) ([]Image, error) {
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	images, err := collection(ctx, c, imagesPath, "ISO inventory", url.Values{"$filter": {"type eq Vmm.Content.ImageType'ISO_IMAGE'"}}, func(i Image) string { return i.ID })
	if err != nil {
		return nil, err
	}
	result := make([]Image, 0, len(images))
	for _, image := range images {
		if image.Type != "ISO_IMAGE" {
			continue
		}
		if image.Locations == nil {
			image, err = c.Image(ctx, image.ID, cluster)
			if err != nil {
				return nil, err
			}
		}
		image.placement(cluster)
		result = append(result, image)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return result, nil
}

func (c *Client) VMState(ctx context.Context, id string) (VM, error) {
	if !uuidPattern.MatchString(id) {
		return VM{}, fault.New(fault.Configuration, "VM identifier must be a UUID.")
	}
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	var r struct {
		Data VM `json:"data"`
	}
	_, err := c.requestOperation(ctx, "GET", vmsPath+"/"+id, nil, "", nil, "VM state read", &r)
	if err != nil {
		return VM{}, err
	}
	if !strings.EqualFold(r.Data.ID, id) || !uuidPattern.MatchString(r.Data.Cluster.ID) {
		return VM{}, fault.New(fault.Protocol, "PC returned malformed selected-VM state.")
	}
	switch r.Data.PowerState {
	case "ON", "OFF", "PAUSED":
	default:
		r.Data.PowerState = "UNKNOWN"
	}
	return r.Data, nil
}

func (c *Client) Power(ctx context.Context, vmID, action string) MutationResult {
	labels := map[string]string{"power-on": "Power on", "power-off": "Power off", "reset": "Restart (hard)", "guest-reboot": "Restart guest OS", "guest-shutdown": "Shut down guest OS"}
	label, ok := labels[action]
	if !ok || !uuidPattern.MatchString(vmID) {
		return MutationResult{"failed", "Invalid selected-VM power action."}
	}
	var body any
	if action == "guest-reboot" || action == "guest-shutdown" {
		body = struct{}{}
	}
	return c.mutation(ctx, vmsPath+"/"+vmID+"/$actions/"+action, body, label)
}

func (c *Client) Media(ctx context.Context, vmID, driveID, imageID string, eject bool) MutationResult {
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	if !uuidPattern.MatchString(driveID) {
		return MutationResult{"failed", "Invalid CD-ROM selection; refresh drives."}
	}
	drives, err := c.Drives(ctx, vmID)
	if err != nil {
		return MutationResult{"failed", err.Error()}
	}
	found := false
	for _, drive := range drives {
		if strings.EqualFold(drive.ID, driveID) {
			found = true
		}
	}
	if !found {
		return MutationResult{"failed", "Selected CD-ROM no longer belongs to this VM; refresh drives."}
	}
	path := vmsPath + "/" + vmID + "/cd-roms/" + driveID + "/$actions/"
	if eject {
		return c.mutation(ctx, path+"eject", nil, "Eject")
	}
	vm, err := c.VMState(ctx, vmID)
	if err != nil {
		return MutationResult{"failed", err.Error()}
	}
	image, err := c.Image(ctx, imageID, vm.Cluster.ID)
	if err != nil {
		return MutationResult{"failed", err.Error()}
	}
	if image.Availability == "unavailable" {
		return MutationResult{"failed", "Selected ISO is unavailable on this VM's cluster; refresh ISO inventory."}
	}
	body := map[string]any{"backingInfo": map[string]any{"dataSource": map[string]any{"reference": map[string]string{"$objectType": "vmm.v4.ahv.config.ImageReference", "imageExtId": image.ID}}}}
	return c.mutation(ctx, path+"insert", body, "Mount ISO")
}
