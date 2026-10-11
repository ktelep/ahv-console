package prism

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ahv-console/internal/fault"
)

// These management responses are synthetic fixtures based on official schemas.
func TestManagementTransport(t *testing.T) {
	var hits atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		u, p, ok := r.BasicAuth()
		if !ok || u != "fixture-user" || p != "fixture-secret" || string(body) != `{}` || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("NTNX-Request-Id") != testVM {
			t.Error("incorrect authenticated JSON request")
		}
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"fixture-secret"}`))
	}))
	defer s.Close()
	c := clientFor(t, s)
	var out any
	_, err := c.requestOperation(context.Background(), "POST", vmsPath, nil, testVM, struct{}{}, "guest restart", &out)
	kind(t, err, fault.Permission)
	if !strings.Contains(err.Error(), "guest restart") || strings.Contains(err.Error(), "Console Token") || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal("incorrect operation diagnostic", err)
	}
	_, err = c.requestOperation(context.Background(), "POST", vmsPath, nil, testVM, strings.Repeat("x", maxRequest), "guest restart", &out)
	kind(t, err, fault.Protocol)
	if hits.Load() != 1 {
		t.Fatal("oversized body reached upstream")
	}
}

func TestObservedGuestTaskFailuresDoNotFallback(t *testing.T) {
	for _, tc := range []struct{ action, file string }{
		{"guest-reboot", "management.guest-reboot-task.observed.json"},
		{"guest-shutdown", "management.guest-shutdown-task.observed.json"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			var posts atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts.Add(1)
					if r.URL.Path != vmsPath+"/"+testVM+"/$actions/"+tc.action {
						t.Error("unexpected hard fallback", r.URL.Path)
					}
					w.WriteHeader(202)
					writeJSON(w, map[string]any{"data": map[string]string{"extId": testTask}})
					return
				}
				w.Write(fixture(t, tc.file))
			}))
			defer s.Close()
			result := clientFor(t, s).Power(context.Background(), testVM, tc.action)
			if result.Status != "failed" || posts.Load() != 1 {
				t.Fatal(result, posts.Load())
			}
		})
	}
}

func TestMediaWireAndStaleSelections(t *testing.T) {
	for _, mode := range []string{"insert", "eject", "stale-drive", "stale-image", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			var posts atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					t.Error("drive deletion")
				}
				if r.Method == "POST" {
					posts.Add(1)
					action := "insert"
					if mode == "eject" {
						action = "eject"
					}
					if r.URL.Path != vmsPath+"/"+testVM+"/cd-roms/"+testCluster+"/$actions/"+action {
						t.Error("wrong target", r.URL.Path)
					}
					b, _ := io.ReadAll(r.Body)
					if mode == "eject" {
						if len(b) != 0 {
							t.Error("eject body")
						}
					} else {
						var body map[string]any
						if json.Unmarshal(b, &body) != nil {
							t.Fatal("body")
						}
						ref := body["backingInfo"].(map[string]any)["dataSource"].(map[string]any)["reference"].(map[string]any)
						if len(ref) != 2 || ref["imageExtId"] != testVM || ref["$objectType"] != "vmm.v4.ahv.config.ImageReference" {
							t.Error(ref)
						}
					}
					w.WriteHeader(202)
					writeJSON(w, map[string]any{"data": map[string]string{"extId": testTask}})
					return
				}
				if strings.Contains(r.URL.Path, "/tasks/") {
					fmt.Fprint(w, `{"data":{"status":"SUCCEEDED"}}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/cd-roms") {
					if mode == "stale-drive" {
						fmt.Fprint(w, `{"data":[],"metadata":{"totalAvailableResults":0}}`)
						return
					}
					fmt.Fprintf(w, `{"data":[{"extId":%q}],"metadata":{"totalAvailableResults":1}}`, testCluster)
					return
				}
				if strings.HasPrefix(r.URL.Path, imagesPath) {
					if mode == "stale-image" {
						w.WriteHeader(404)
						return
					}
					locations := []string{testCluster}
					if mode == "unavailable" {
						locations = []string{}
					}
					writeJSON(w, map[string]any{"data": Image{ID: testVM, Type: "ISO_IMAGE", Locations: locations}})
					return
				}
				var vm VM
				vm.ID = testVM
				vm.Cluster.ID = testCluster
				writeJSON(w, map[string]any{"data": vm})
			}))
			defer s.Close()
			result := clientFor(t, s).Media(context.Background(), testVM, testCluster, testVM, mode == "eject")
			if mode == "insert" || mode == "eject" {
				if result.Status != "succeeded" || posts.Load() != 1 {
					t.Fatal(result)
				}
			} else if result.Status != "failed" || posts.Load() != 0 {
				t.Fatal(result, posts.Load())
			}
		})
	}
}

func TestInventory(t *testing.T) {
	for _, mode := range []string{"pages", "repeat", "incomplete", "denied", "empty", "drives", "absent"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := r.URL.Query().Get("$page")
				if mode == "denied" {
					w.WriteHeader(403)
					return
				}
				if mode == "empty" || mode == "absent" {
					fmt.Fprint(w, `{"metadata":{"totalAvailableResults":0}}`)
					return
				}
				if mode == "drives" {
					fmt.Fprintf(w, `{"data":[{"extId":%q,"diskAddress":{"busType":"SATA","index":2},"backingInfo":{"diskExtId":%q}},{"extId":%q,"diskAddress":{"busType":"IDE","index":0}}],"metadata":{"totalAvailableResults":2}}`, testVM, testVM, testCluster)
					return
				}
				if r.URL.Query().Get("$filter") != "type eq Vmm.Content.ImageType'ISO_IMAGE'" {
					t.Error("missing ISO filter")
				}
				if page == "0" || mode == "repeat" {
					fmt.Fprintf(w, `{"data":[{"extId":%q,"name":"same","type":"ISO_IMAGE","sizeBytes":42,"clusterLocationExtIds":[%q]}],"metadata":{"totalAvailableResults":3}}`, testVM, testCluster)
					return
				}
				if mode == "incomplete" {
					fmt.Fprint(w, `{"data":[],"metadata":{"totalAvailableResults":3}}`)
					return
				}
				fmt.Fprintf(w, `{"data":[{"extId":%q,"name":"same","type":"ISO_IMAGE","clusterLocationExtIds":[]},{"extId":%q,"name":"disk","type":"DISK_IMAGE"}],"metadata":{"totalAvailableResults":3}}`, testCluster, "00000000-0000-4000-8000-000000000004")
			}))
			defer s.Close()
			c := clientFor(t, s)
			if mode == "drives" || mode == "absent" {
				drives, err := c.Drives(context.Background(), testVM)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "drives" && (len(drives) != 2 || drives[0].Address.Bus != "IDE" || drives[1].Backing == nil) {
					t.Fatal(drives)
				}
				if mode == "absent" && len(drives) != 0 {
					t.Fatal(drives)
				}
				return
			}
			images, err := c.Images(context.Background(), testCluster)
			if mode == "denied" {
				kind(t, err, fault.Permission)
				return
			}
			if mode == "repeat" || mode == "incomplete" {
				kind(t, err, fault.Protocol)
				if images != nil {
					t.Fatal("partial inventory")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "pages" && (len(images) != 2 || images[0].ID != testVM || images[0].Availability != "ready" || images[1].Availability != "unavailable") {
				t.Fatal(images)
			}
			if mode == "empty" && len(images) != 0 {
				t.Fatal(images)
			}
		})
	}
}

func TestImageDetailUnknownPlacement(t *testing.T) {
	var details atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == imagesPath {
			fmt.Fprintf(w, `{"data":[{"extId":%q,"name":"ISO","type":"ISO_IMAGE"}],"metadata":{"totalAvailableResults":1}}`, testVM)
			return
		}
		details.Add(1)
		fmt.Fprintf(w, `{"data":{"extId":%q,"name":"ISO","type":"ISO_IMAGE"}}`, testVM)
	}))
	defer s.Close()
	images, err := clientFor(t, s).Images(context.Background(), testCluster)
	if err != nil || len(images) != 1 || images[0].Availability != "unknown" || details.Load() != 1 {
		t.Fatal(images, err)
	}
}

func TestPowerMappingsAndState(t *testing.T) {
	for _, action := range []string{"power-on", "power-off", "reset", "guest-reboot", "guest-shutdown"} {
		t.Run(action, func(t *testing.T) {
			var posts atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts.Add(1)
					if r.URL.Path != vmsPath+"/"+testVM+"/$actions/"+action {
						t.Error("wrong action", r.URL.Path)
					}
					b, _ := io.ReadAll(r.Body)
					if strings.HasPrefix(action, "guest-") {
						if string(b) != `{}` {
							t.Error("guest body", string(b))
						}
						w.WriteHeader(403)
						return
					}
					if len(b) != 0 {
						t.Error("hard action body")
					}
					w.WriteHeader(202)
					writeJSON(w, map[string]any{"data": map[string]string{"extId": testTask}})
					return
				}
				if strings.Contains(r.URL.Path, "/tasks/") {
					writeJSON(w, map[string]any{"data": map[string]string{"status": "SUCCEEDED"}})
					return
				}
				var vm VM
				vm.ID = testVM
				vm.Cluster.ID = testCluster
				vm.PowerState = "OFF"
				writeJSON(w, map[string]any{"data": vm})
			}))
			defer s.Close()
			c := clientFor(t, s)
			vm, err := c.VMState(context.Background(), testVM)
			if err != nil || vm.PowerState != "OFF" {
				t.Fatal(vm, err)
			}
			result := c.Power(context.Background(), testVM, action)
			want := "succeeded"
			if strings.HasPrefix(action, "guest-") {
				want = "failed"
			}
			if result.Status != want || posts.Load() != 1 {
				t.Fatal("fallback or incorrect result", result, posts.Load())
			}
		})
	}
}

func TestManagementTasks(t *testing.T) {
	for _, tc := range []struct{ upstream, want string }{
		{"SUCCEEDED", "succeeded"}, {"FAILED", "failed"}, {"CANCELED", "canceled"}, {"RUNNING", "unconfirmed"}, {"$UNKNOWN", "unconfirmed"}, {"denied", "failed"}, {"lost", "unconfirmed"},
	} {
		t.Run(tc.upstream, func(t *testing.T) {
			var posts atomic.Int32
			var requestID string
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					n := posts.Add(1)
					id := r.Header.Get("NTNX-Request-Id")
					if n == 1 {
						requestID = id
					} else if id != requestID {
						t.Error("request ID changed")
					}
					if tc.upstream == "denied" {
						w.WriteHeader(403)
						return
					}
					if tc.upstream == "lost" || n == 1 {
						w.WriteHeader(503)
						return
					}
					w.WriteHeader(202)
					writeJSON(w, map[string]any{"data": map[string]string{"extId": testTask}})
					return
				}
				if r.Header.Get("NTNX-Request-Id") != "" {
					t.Error("poll used mutation ID")
				}
				writeJSON(w, map[string]any{"data": map[string]string{"status": tc.upstream}})
			}))
			defer s.Close()
			c := clientFor(t, s)
			c.setupTimeout = 50 * time.Millisecond
			result := c.mutation(context.Background(), vmsPath+"/"+testVM+"/$actions/reset", nil, "hard restart")
			if result.Status != tc.want {
				t.Fatal(result)
			}
			wantPosts := int32(2)
			if tc.upstream == "denied" {
				wantPosts = 1
			}
			if tc.upstream == "lost" {
				wantPosts = 3
			}
			if posts.Load() != wantPosts {
				t.Fatal("automatic replay", posts.Load())
			}
		})
	}
}
