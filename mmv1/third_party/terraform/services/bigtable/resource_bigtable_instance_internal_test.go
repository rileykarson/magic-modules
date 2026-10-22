package bigtable

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/bigtable"
	btapb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestUnitBigtable_getUnavailableClusterZones(t *testing.T) {
	cases := map[string]struct {
		clusterZones     []string
		unavailableZones []string
		want             []string
	}{
		"not unavailalbe": {
			clusterZones:     []string{"us-central1", "eu-west1"},
			unavailableZones: []string{"us-central2", "eu-west2"},
			want:             nil,
		},
		"unavailable one to one": {
			clusterZones:     []string{"us-central2"},
			unavailableZones: []string{"us-central2"},
			want:             []string{"us-central2"},
		},
		"unavailable one to many": {
			clusterZones:     []string{"us-central2"},
			unavailableZones: []string{"us-central2", "us-central1"},
			want:             []string{"us-central2"},
		},
		"unavailable many to one": {
			clusterZones:     []string{"us-central2", "us-central1"},
			unavailableZones: []string{"us-central2"},
			want:             []string{"us-central2"},
		},
		"unavailable many to many": {
			clusterZones:     []string{"us-central2", "us-central1"},
			unavailableZones: []string{"us-central2", "us-central1"},
			want:             []string{"us-central2", "us-central1"},
		},
	}

	for tn, tc := range cases {
		var clusters []interface{}
		for _, zone := range tc.clusterZones {
			clusters = append(clusters, map[string]interface{}{"zone": zone})
		}
		if got := getUnavailableClusterZones(clusters, tc.unavailableZones); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("bad: %s, got %q, want %q", tn, got, tc.want)
		}
	}
}

func TestUnitBigtable_getInstanceFromResponse(t *testing.T) {
	instanceName := "test-instance"
	originalId := "original_value"
	cases := map[string]struct {
		instanceNames      []string
		listInstancesError error

		wantError        string
		wantInstanceName string
		wantStop         bool
		wantId           string
	}{
		"not found": {
			instanceNames:      []string{"wrong", "also_wrong"},
			listInstancesError: nil,

			wantError:        "",
			wantStop:         true,
			wantInstanceName: "",
			wantId:           "",
		},
		"found": {
			instanceNames:      []string{"wrong", "also_wrong", instanceName},
			listInstancesError: nil,

			wantError:        "",
			wantStop:         false,
			wantInstanceName: instanceName,
			wantId:           originalId,
		},
		"error": {
			instanceNames:      nil,
			listInstancesError: fmt.Errorf("some error"),

			wantError:        "Error retrieving instance.",
			wantStop:         true,
			wantInstanceName: "",
			wantId:           originalId,
		},
		"unavailable error": {
			instanceNames:      []string{"wrong", "also_wrong"},
			listInstancesError: bigtable.ErrPartiallyUnavailable{Locations: []string{"some", "location"}},

			wantError:        "",
			wantStop:         false,
			wantInstanceName: "",
			wantId:           originalId,
		}}
	for tn, tc := range cases {
		instancesResponse := []*bigtable.InstanceInfo{}
		for _, existingInstance := range tc.instanceNames {
			instancesResponse = append(instancesResponse, &bigtable.InstanceInfo{Name: existingInstance})
		}
		d := &schema.ResourceData{}
		d.SetId(originalId)
		gotInstance, gotStop, gotErr := getInstanceFromResponse(instancesResponse, instanceName, tc.listInstancesError, d)

		if gotStop != tc.wantStop {
			t.Errorf("bad stop: %s, got %v, want %v", tn, gotStop, tc.wantStop)
		}
		if (gotErr != nil && tc.wantError == "") ||
			(gotErr == nil && tc.wantError != "") ||
			(gotErr != nil && !strings.Contains(gotErr.Error(), tc.wantError)) {
			t.Errorf("bad error: %s, got %q, want %q", tn, gotErr, tc.wantError)
		}
		if (gotInstance == nil && tc.wantInstanceName != "") ||
			(gotInstance != nil && tc.wantInstanceName == "") ||
			(gotInstance != nil && gotInstance.Name != tc.wantInstanceName) {
			t.Errorf("bad instance: %s, got %v, want %q", tn, gotInstance, tc.wantInstanceName)
		}
		gotId := d.Id()
		if gotId != tc.wantId {
			t.Errorf("bad ID: %s, got %v, want %q", tn, gotId, tc.wantId)
		}
	}
}

func TestUnitBigtable_flattenBigtableCluster(t *testing.T) {
	cases := map[string]struct {
		clusterInfo *bigtable.ClusterInfo
		want        map[string]interface{}
	}{
		"SSD auto scaling": {
			clusterInfo: &bigtable.ClusterInfo{
				StorageType: bigtable.SSD,
				Zone:        "zone1",
				ServeNodes:  5,
				Name:        "ssd-cluster",
				KMSKeyName:  "KMS",
				State:       "CREATING",
				AutoscalingConfig: &bigtable.AutoscalingConfig{
					MinNodes:                  3,
					MaxNodes:                  7,
					CPUTargetPercent:          50,
					StorageUtilizationPerNode: 60,
				},
			},
			want: map[string]interface{}{
				"zone":         "zone1",
				"num_nodes":    5,
				"cluster_id":   "ssd-cluster",
				"storage_type": "SSD",
				"kms_key_name": "KMS",
				"state":        "CREATING",
				"autoscaling_config": []map[string]interface{}{
					{
						"min_nodes":      3,
						"max_nodes":      7,
						"cpu_target":     50,
						"storage_target": 60,
					},
				},
				// unspecified node scaling factor in input will default to 1X
				"node_scaling_factor": "NodeScalingFactor1X",
			},
		},
		"HDD manual scaling": {
			clusterInfo: &bigtable.ClusterInfo{
				StorageType:       bigtable.HDD,
				Zone:              "zone2",
				ServeNodes:        7,
				Name:              "hdd-cluster",
				KMSKeyName:        "KMS",
				State:             "READY",
				NodeScalingFactor: bigtable.NodeScalingFactor2X,
			},
			want: map[string]interface{}{
				"zone":                "zone2",
				"num_nodes":           7,
				"cluster_id":          "hdd-cluster",
				"storage_type":        "HDD",
				"kms_key_name":        "KMS",
				"state":               "READY",
				"node_scaling_factor": "NodeScalingFactor2X",
			},
		},
	}

	for tn, tc := range cases {
		if got := flattenBigtableCluster(tc.clusterInfo); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("bad: %s, got %q, want %q", tn, got, tc.want)
		}
	}
}

func TestUnitBigtable_resourceBigtableInstanceClusterReorderTypeListFunc_error(t *testing.T) {
	d := &tpgresource.ResourceDiffMock{
		After: map[string]interface{}{
			"cluster.#": 0,
		},
	}
	if err := resourceBigtableInstanceClusterReorderTypeListFunc(d, nil); err == nil {
		t.Errorf("expected error, got success")
	}
}

func TestUnitBigtable_resourceBigtableInstanceClusterReorderTypeListFunc(t *testing.T) {
	cases := map[string]struct {
		before           map[string]interface{}
		after            map[string]interface{}
		wantClusterOrder []string
		wantForceNew     bool
	}{
		"create": {
			before: map[string]interface{}{
				"cluster.#":            1,
				"cluster.0.cluster_id": "some-id-a",
			},
			after: map[string]interface{}{
				"name":                 "some-name",
				"cluster.#":            1,
				"cluster.0.cluster_id": "some-id-a",
				"cluster.0": map[string]interface{}{
					"cluster_id": "some-id-a",
				},
			},
			wantClusterOrder: []string{},
			wantForceNew:     false,
		},
		"no force new change": {
			before: map[string]interface{}{
				"name":                 "some-name",
				"cluster.#":            4,
				"cluster.0.cluster_id": "some-id-a",
				"cluster.1.cluster_id": "some-id-b",
				"cluster.2.cluster_id": "some-id-c",
				"cluster.3.cluster_id": "some-id-e",
			},
			after: map[string]interface{}{
				"name":                 "some-name",
				"cluster.#":            3,
				"cluster.0.cluster_id": "some-id-c",
				"cluster.1.cluster_id": "some-id-a",
				"cluster.2.cluster_id": "some-id-d",
				"cluster.0": map[string]interface{}{
					"cluster_id": "some-id-c",
				},
				"cluster.1": map[string]interface{}{
					"cluster_id": "some-id-a",
				},
				"cluster.2": map[string]interface{}{
					"cluster_id": "some-id-d",
				},
			},
			wantClusterOrder: []string{"some-id-a", "some-id-d", "some-id-c"},
			wantForceNew:     false,
		},
		"force new - zone change": {
			before: map[string]interface{}{
				"name":                 "some-name",
				"cluster.#":            1,
				"cluster.0.cluster_id": "some-id-a",
				"cluster.0.zone":       "zone-a",
			},
			after: map[string]interface{}{
				"name":                 "some-name",
				"cluster.#":            1,
				"cluster.0.cluster_id": "some-id-a",
				"cluster.0.zone":       "zone-b",
				"cluster.0": map[string]interface{}{
					"cluster_id": "some-id-a",
					"zone":       "zone-b",
				},
			},
			wantClusterOrder: []string{"some-id-a"},
			wantForceNew:     true,
		},
		"force new - kms_key_name change": {
			before: map[string]interface{}{
				"name":                   "some-name",
				"cluster.#":              1,
				"cluster.0.cluster_id":   "some-id-a",
				"cluster.0.kms_key_name": "key-a",
			},
			after: map[string]interface{}{
				"name":                   "some-name",
				"cluster.#":              1,
				"cluster.0.cluster_id":   "some-id-a",
				"cluster.0.kms_key_name": "key-b",
				"cluster.0": map[string]interface{}{
					"cluster_id":   "some-id-a",
					"kms_key_name": "key-b",
				},
			},
			wantClusterOrder: []string{"some-id-a"},
			wantForceNew:     true,
		},
		"force new - storage_type change": {
			before: map[string]interface{}{
				"name":                   "some-name",
				"cluster.#":              1,
				"cluster.0.cluster_id":   "some-id-a",
				"cluster.0.storage_type": "HDD",
				"cluster.0.state":        "READY",
			},
			after: map[string]interface{}{
				"name":                   "some-name",
				"cluster.#":              1,
				"cluster.0.cluster_id":   "some-id-a",
				"cluster.0.storage_type": "SSD",
				"cluster.0": map[string]interface{}{
					"cluster_id":   "some-id-a",
					"storage_type": "SSD",
				},
			},
			wantClusterOrder: []string{"some-id-a"},
			wantForceNew:     true,
		},
		"skip force new - storage_type change for CREATING cluster": {
			before: map[string]interface{}{
				"name":                   "some-name",
				"cluster.#":              1,
				"cluster.0.cluster_id":   "some-id-a",
				"cluster.0.storage_type": "SSD",
				"cluster.0.state":        "CREATING",
			},
			after: map[string]interface{}{
				"name":                   "some-name",
				"cluster.#":              1,
				"cluster.0.cluster_id":   "some-id-a",
				"cluster.0.storage_type": "HDD",
				"cluster.0": map[string]interface{}{
					"cluster_id":   "some-id-a",
					"storage_type": "HDD",
				},
			},
			wantClusterOrder: []string{"some-id-a"},
			wantForceNew:     false,
		},
	}
	for tn, tc := range cases {
		t.Run(tn, func(t *testing.T) {
			d := &tpgresource.ResourceDiffMock{
				Before: tc.before,
				After:  tc.after,
			}
			var clusters []interface{}
			err := resourceBigtableInstanceClusterReorderTypeListFunc(d, func(gotClusters []interface{}) error {
				clusters = gotClusters
				return nil
			})
			if err != nil {
				t.Fatalf("bad: %s, error: %v", tn, err)
			}
			if d.IsForceNew != tc.wantForceNew {
				t.Errorf("bad: %s, got %v, want %v", tn, d.IsForceNew, tc.wantForceNew)
			}
			gotClusterOrder := []string{}
			for _, cluster := range clusters {
				clusterResource := cluster.(map[string]interface{})
				gotClusterOrder = append(gotClusterOrder, clusterResource["cluster_id"].(string))
			}
			if !reflect.DeepEqual(gotClusterOrder, tc.wantClusterOrder) {
				t.Errorf("bad: %s, got %q, want %q", tn, gotClusterOrder, tc.wantClusterOrder)
			}
		})
	}
}

type mockBigtableInstanceAdminClient struct {
	existingClusters   []*bigtable.ClusterInfo
	clustersErr        error
	updateInstanceErr  error
	setAutoscalingErr  error
	updateClusterErr   error
	deleteClusterErr   error
	createClusterErr   error
	lastInstanceConfig *bigtable.InstanceWithClustersConfig
	ops                []string
}

func (m *mockBigtableInstanceAdminClient) Clusters(ctx context.Context, instanceID string) ([]*bigtable.ClusterInfo, error) {
	m.ops = append(m.ops, fmt.Sprintf("Clusters(%s)", instanceID))
	if m.clustersErr != nil {
		return m.existingClusters, m.clustersErr
	}
	return m.existingClusters, nil
}

func (m *mockBigtableInstanceAdminClient) UpdateInstanceWithClusters(ctx context.Context, conf *bigtable.InstanceWithClustersConfig) error {
	cp := *conf
	m.lastInstanceConfig = &cp
	m.ops = append(m.ops, fmt.Sprintf("UpdateInstanceWithClusters(%s,clusters=%d)", conf.InstanceID, len(conf.Clusters)))
	return m.updateInstanceErr
}

func (m *mockBigtableInstanceAdminClient) SetAutoscaling(ctx context.Context, instanceID, clusterID string, conf bigtable.AutoscalingConfig) error {
	m.ops = append(m.ops, fmt.Sprintf("SetAutoscaling(%s,%s,%d-%d,%d,%d)", instanceID, clusterID, conf.MinNodes, conf.MaxNodes, conf.CPUTargetPercent, conf.StorageUtilizationPerNode))
	return m.setAutoscalingErr
}

func (m *mockBigtableInstanceAdminClient) UpdateCluster(ctx context.Context, instanceID, clusterID string, serveNodes int32) error {
	m.ops = append(m.ops, fmt.Sprintf("UpdateCluster(%s,%s,%d)", instanceID, clusterID, serveNodes))
	return m.updateClusterErr
}

func (m *mockBigtableInstanceAdminClient) DeleteCluster(ctx context.Context, instanceID, clusterID string) error {
	m.ops = append(m.ops, fmt.Sprintf("DeleteCluster(%s,%s)", instanceID, clusterID))
	return m.deleteClusterErr
}

func (m *mockBigtableInstanceAdminClient) CreateCluster(ctx context.Context, conf *bigtable.ClusterConfig) error {
	m.ops = append(m.ops, fmt.Sprintf("CreateCluster(%s,%s,%s,%d)", conf.InstanceID, conf.ClusterID, conf.Zone, conf.NumNodes))
	return m.createClusterErr
}

func TestUnitBigtable_updateInstanceAndSyncClusters(t *testing.T) {
	cases := map[string]struct {
		mock        *mockBigtableInstanceAdminClient
		conf        *bigtable.InstanceWithClustersConfig
		wantResults *bigtable.UpdateInstanceResults
		wantOps     []string
		wantErr     string
	}{
		"update instance and existing cluster manual scaling": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}},
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID:   "inst-1",
				DisplayName:  "Instance 1",
				InstanceType: bigtable.PRODUCTION,
				Edition:      bigtable.EnterprisePlus,
				Labels:       map[string]string{"env": "prod"},
				Clusters: []bigtable.ClusterConfig{
					{ClusterID: "c1", NumNodes: 3},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: true,
				UpdatedClusters: []string{"c1"},
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"UpdateCluster(inst-1,c1,3)",
			},
		},
		"autoscaling takes precedence over num_nodes and no instance fields updated": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}, {Name: "c2"}},
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID: "inst-1",
				Clusters: []bigtable.ClusterConfig{
					{
						ClusterID: "c1",
						NumNodes:  5,
						AutoscalingConfig: &bigtable.AutoscalingConfig{
							MinNodes:                  2,
							MaxNodes:                  6,
							CPUTargetPercent:          70,
							StorageUtilizationPerNode: 2560,
						},
					},
					{
						// Zero nodes and no autoscaling config: should not be updated
						ClusterID: "c2",
					},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: false,
				UpdatedClusters: []string{"c1"},
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"SetAutoscaling(inst-1,c1,2-6,70,2560)",
			},
		},
		"replace sole cluster creates new cluster before deleting old cluster": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}},
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID:  "inst-1",
				DisplayName: "inst-1",
				Clusters: []bigtable.ClusterConfig{
					{ClusterID: "c2", Zone: "us-central1-b", NumNodes: 3},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: true,
				CreatedClusters: []string{"c2"},
				DeletedClusters: []string{"c1"},
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"CreateCluster(inst-1,c2,us-central1-b,3)",
				"DeleteCluster(inst-1,c1)",
			},
		},
		"replace cluster when multiple exist deletes before creating": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}, {Name: "c2"}},
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID: "inst-1",
				Clusters: []bigtable.ClusterConfig{
					{ClusterID: "c1", NumNodes: 4},
					{ClusterID: "c3", Zone: "us-east1-b", NumNodes: 2},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: false,
				UpdatedClusters: []string{"c1"},
				DeletedClusters: []string{"c2"},
				CreatedClusters: []string{"c3"},
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"UpdateCluster(inst-1,c1,4)",
				"DeleteCluster(inst-1,c2)",
				"CreateCluster(inst-1,c3,us-east1-b,2)",
			},
		},
		"error listing clusters": {
			mock: &mockBigtableInstanceAdminClient{
				clustersErr: bigtable.ErrPartiallyUnavailable{Locations: []string{"us-central1-a"}},
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID: "inst-1",
			},
			wantResults: nil,
			wantOps: []string{
				"Clusters(inst-1)",
			},
			wantErr: "Unavailable locations: [us-central1-a]",
		},
		"error updating instance": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters:  []*bigtable.ClusterInfo{{Name: "c1"}},
				updateInstanceErr: errors.New("instance update boom"),
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID:  "inst-1",
				DisplayName: "new-name",
			},
			wantResults: nil,
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
			},
			wantErr: "instance update boom",
		},
		"error updating cluster returns partial results and progress message": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}},
				updateClusterErr: errors.New("cluster update boom"),
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID:  "inst-1",
				DisplayName: "new-name",
				Clusters: []bigtable.ClusterConfig{
					{ClusterID: "c1", NumNodes: 3},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: true,
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"UpdateCluster(inst-1,c1,3)",
			},
			wantErr: `UpdateCluster "c1" failed cluster update boom; Progress: Instance updated? true Clusters added:[] Clusters deleted:[] Clusters updated:[]`,
		},
		"error deleting cluster returns partial results and progress message": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}, {Name: "c2"}},
				deleteClusterErr: errors.New("delete boom"),
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID: "inst-1",
				Clusters: []bigtable.ClusterConfig{
					{ClusterID: "c1", NumNodes: 2},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: false,
				UpdatedClusters: []string{"c1"},
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"UpdateCluster(inst-1,c1,2)",
				"DeleteCluster(inst-1,c2)",
			},
			wantErr: `DeleteCluster "c2" failed delete boom; Progress: Instance updated? false Clusters added:[] Clusters deleted:[] Clusters updated:[c1]`,
		},
		"error creating cluster returns partial results and progress message": {
			mock: &mockBigtableInstanceAdminClient{
				existingClusters: []*bigtable.ClusterInfo{{Name: "c1"}},
				createClusterErr: errors.New("create boom"),
			},
			conf: &bigtable.InstanceWithClustersConfig{
				InstanceID: "inst-1",
				Clusters: []bigtable.ClusterConfig{
					{ClusterID: "c1", NumNodes: 2},
					{ClusterID: "c2", Zone: "us-central1-b", NumNodes: 3},
				},
			},
			wantResults: &bigtable.UpdateInstanceResults{
				InstanceUpdated: false,
				UpdatedClusters: []string{"c1"},
			},
			wantOps: []string{
				"Clusters(inst-1)",
				"UpdateInstanceWithClusters(inst-1,clusters=0)",
				"UpdateCluster(inst-1,c1,2)",
				"CreateCluster(inst-1,c2,us-central1-b,3)",
			},
			wantErr: "CreateCluster {inst-1 c2 us-central1-b 3 0  <nil> 0} failed create boom; Progress: Instance updated? false Clusters added:[] Clusters deleted:[] Clusters updated:[c1]",
		},
	}

	for tn, tc := range cases {
		t.Run(tn, func(t *testing.T) {
			gotResults, err := updateInstanceAndSyncClusters(context.Background(), tc.mock, tc.conf)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("got error %v, want %q", err, tc.wantErr)
				}
			}
			if !reflect.DeepEqual(gotResults, tc.wantResults) {
				t.Errorf("got results %+v, want %+v", gotResults, tc.wantResults)
			}
			if !reflect.DeepEqual(tc.mock.ops, tc.wantOps) {
				t.Errorf("got ops %v, want %v", tc.mock.ops, tc.wantOps)
			}
		})
	}
}

type fakeBigtableInstanceAdminServer struct {
	btapb.UnimplementedBigtableInstanceAdminServer

	clusters                  []*btapb.Cluster
	partialUpdateInstanceReqs []*btapb.PartialUpdateInstanceRequest
	partialUpdateClusterReqs  []*btapb.PartialUpdateClusterRequest
	createClusterReqs         []*btapb.CreateClusterRequest
	deleteClusterReqs         []*btapb.DeleteClusterRequest
}

func (s *fakeBigtableInstanceAdminServer) ListClusters(ctx context.Context, req *btapb.ListClustersRequest) (*btapb.ListClustersResponse, error) {
	return &btapb.ListClustersResponse{Clusters: s.clusters}, nil
}

func (s *fakeBigtableInstanceAdminServer) PartialUpdateInstance(ctx context.Context, req *btapb.PartialUpdateInstanceRequest) (*longrunningpb.Operation, error) {
	s.partialUpdateInstanceReqs = append(s.partialUpdateInstanceReqs, req)
	return &longrunningpb.Operation{
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: &anypb.Any{}},
	}, nil
}

func (s *fakeBigtableInstanceAdminServer) PartialUpdateCluster(ctx context.Context, req *btapb.PartialUpdateClusterRequest) (*longrunningpb.Operation, error) {
	s.partialUpdateClusterReqs = append(s.partialUpdateClusterReqs, req)
	return &longrunningpb.Operation{
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: &anypb.Any{}},
	}, nil
}

func (s *fakeBigtableInstanceAdminServer) CreateCluster(ctx context.Context, req *btapb.CreateClusterRequest) (*longrunningpb.Operation, error) {
	s.createClusterReqs = append(s.createClusterReqs, req)
	respAny, err := anypb.New(&btapb.Cluster{})
	if err != nil {
		return nil, err
	}
	return &longrunningpb.Operation{
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: respAny},
	}, nil
}

func (s *fakeBigtableInstanceAdminServer) DeleteCluster(ctx context.Context, req *btapb.DeleteClusterRequest) (*emptypb.Empty, error) {
	s.deleteClusterReqs = append(s.deleteClusterReqs, req)
	return &emptypb.Empty{}, nil
}

func TestUnitBigtable_UpdateInstanceAndSyncClusters_GRPC(t *testing.T) {
	ctx := context.Background()
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })

	srv := &fakeBigtableInstanceAdminServer{
		clusters: []*btapb.Cluster{
			{
				Name:               "projects/test-proj/instances/test-inst/clusters/c1",
				Location:           "projects/test-proj/locations/us-central1-a",
				State:              btapb.Cluster_READY,
				ServeNodes:         1,
				DefaultStorageType: btapb.StorageType_SSD,
			},
			{
				Name:               "projects/test-proj/instances/test-inst/clusters/c2",
				Location:           "projects/test-proj/locations/us-central1-b",
				State:              btapb.Cluster_READY,
				ServeNodes:         1,
				DefaultStorageType: btapb.StorageType_SSD,
			},
		},
	}

	grpcServer := grpc.NewServer()
	btapb.RegisterBigtableInstanceAdminServer(grpcServer, srv)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(func() { grpcServer.Stop() })

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.DialContext failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	client, err := bigtable.NewInstanceAdminClient(ctx, "test-proj", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewInstanceAdminClient failed: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	conf := &bigtable.InstanceWithClustersConfig{
		InstanceID:   "test-inst",
		DisplayName:  "Updated Display",
		InstanceType: bigtable.PRODUCTION,
		Edition:      bigtable.EnterprisePlus,
		Labels:       map[string]string{"k": "v"},
		Clusters: []bigtable.ClusterConfig{
			{
				ClusterID: "c1",
				Zone:      "us-central1-a",
				AutoscalingConfig: &bigtable.AutoscalingConfig{
					MinNodes:                  1,
					MaxNodes:                  3,
					CPUTargetPercent:          50,
					StorageUtilizationPerNode: 2560,
				},
			},
			{
				ClusterID:   "c3",
				Zone:        "us-central1-c",
				NumNodes:    2,
				StorageType: bigtable.SSD,
			},
		},
	}

	results, err := UpdateInstanceAndSyncClusters(ctx, client, conf)
	if err != nil {
		t.Fatalf("UpdateInstanceAndSyncClusters failed: %v", err)
	}

	wantResults := &bigtable.UpdateInstanceResults{
		InstanceUpdated: true,
		UpdatedClusters: []string{"c1"},
		DeletedClusters: []string{"c2"},
		CreatedClusters: []string{"c3"},
	}
	if !reflect.DeepEqual(results, wantResults) {
		t.Errorf("got results %+v, want %+v", results, wantResults)
	}

	if len(srv.partialUpdateInstanceReqs) != 1 {
		t.Fatalf("got %d PartialUpdateInstance requests, want 1", len(srv.partialUpdateInstanceReqs))
	}
	instReq := srv.partialUpdateInstanceReqs[0]
	wantMask := []string{"display_name", "type", "edition", "labels"}
	if !reflect.DeepEqual(instReq.UpdateMask.GetPaths(), wantMask) {
		t.Errorf("got instance update mask %v, want %v", instReq.UpdateMask.GetPaths(), wantMask)
	}
	if len(srv.partialUpdateClusterReqs) != 1 {
		t.Fatalf("got %d PartialUpdateCluster requests, want 1", len(srv.partialUpdateClusterReqs))
	}
	if len(srv.deleteClusterReqs) != 1 || srv.deleteClusterReqs[0].GetName() != "projects/test-proj/instances/test-inst/clusters/c2" {
		t.Errorf("unexpected DeleteCluster requests: %+v", srv.deleteClusterReqs)
	}
	if len(srv.createClusterReqs) != 1 || srv.createClusterReqs[0].GetClusterId() != "c3" {
		t.Errorf("unexpected CreateCluster requests: %+v", srv.createClusterReqs)
	}
}
