package dataplex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	dcl "github.com/hashicorp/terraform-provider-google/google/tpgdclresource"
	"github.com/hashicorp/terraform-provider-google/google/tpgdclresource/operations"
)

func (r *Asset) validate() error {

	if err := dcl.Required(r, "name"); err != nil {
		return err
	}
	if err := dcl.Required(r, "resourceSpec"); err != nil {
		return err
	}
	if err := dcl.Required(r, "discoverySpec"); err != nil {
		return err
	}
	if err := dcl.RequiredParameter(r.Project, "Project"); err != nil {
		return err
	}
	if err := dcl.RequiredParameter(r.Location, "Location"); err != nil {
		return err
	}
	if err := dcl.RequiredParameter(r.Lake, "Lake"); err != nil {
		return err
	}
	if err := dcl.RequiredParameter(r.DataplexZone, "DataplexZone"); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(r.ResourceSpec) {
		if err := r.ResourceSpec.validate(); err != nil {
			return err
		}
	}
	if !dcl.IsEmptyValueIndirect(r.ResourceStatus) {
		if err := r.ResourceStatus.validate(); err != nil {
			return err
		}
	}
	if !dcl.IsEmptyValueIndirect(r.SecurityStatus) {
		if err := r.SecurityStatus.validate(); err != nil {
			return err
		}
	}
	if !dcl.IsEmptyValueIndirect(r.DiscoverySpec) {
		if err := r.DiscoverySpec.validate(); err != nil {
			return err
		}
	}
	if !dcl.IsEmptyValueIndirect(r.DiscoveryStatus) {
		if err := r.DiscoveryStatus.validate(); err != nil {
			return err
		}
	}
	return nil
}
func (r *AssetResourceSpec) validate() error {
	if err := dcl.Required(r, "type"); err != nil {
		return err
	}
	return nil
}
func (r *AssetResourceStatus) validate() error {
	return nil
}
func (r *AssetSecurityStatus) validate() error {
	return nil
}
func (r *AssetDiscoverySpec) validate() error {
	if err := dcl.Required(r, "enabled"); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(r.CsvOptions) {
		if err := r.CsvOptions.validate(); err != nil {
			return err
		}
	}
	if !dcl.IsEmptyValueIndirect(r.JsonOptions) {
		if err := r.JsonOptions.validate(); err != nil {
			return err
		}
	}
	return nil
}
func (r *AssetDiscoverySpecCsvOptions) validate() error {
	return nil
}
func (r *AssetDiscoverySpecJsonOptions) validate() error {
	return nil
}
func (r *AssetDiscoveryStatus) validate() error {
	if !dcl.IsEmptyValueIndirect(r.Stats) {
		if err := r.Stats.validate(); err != nil {
			return err
		}
	}
	return nil
}
func (r *AssetDiscoveryStatusStats) validate() error {
	return nil
}
func (r *Asset) basePath() string {
	params := map[string]interface{}{}
	return dcl.Nprintf("https://dataplex.googleapis.com/v1/", params)
}

func (r *Asset) getURL(userBasePath string) (string, error) {
	nr := r.urlNormalized()
	params := map[string]interface{}{
		"project":      dcl.ValueOrEmptyString(nr.Project),
		"location":     dcl.ValueOrEmptyString(nr.Location),
		"dataplexZone": dcl.ValueOrEmptyString(nr.DataplexZone),
		"lake":         dcl.ValueOrEmptyString(nr.Lake),
		"name":         dcl.ValueOrEmptyString(nr.Name),
	}
	return dcl.URL("projects/{{project}}/locations/{{location}}/lakes/{{lake}}/zones/{{dataplexZone}}/assets/{{name}}", nr.basePath(), userBasePath, params), nil
}

func (r *Asset) listURL(userBasePath string) (string, error) {
	nr := r.urlNormalized()
	params := map[string]interface{}{
		"project":      dcl.ValueOrEmptyString(nr.Project),
		"location":     dcl.ValueOrEmptyString(nr.Location),
		"dataplexZone": dcl.ValueOrEmptyString(nr.DataplexZone),
		"lake":         dcl.ValueOrEmptyString(nr.Lake),
	}
	return dcl.URL("projects/{{project}}/locations/{{location}}/lakes/{{lake}}/zones/{{dataplexZone}}/assets", nr.basePath(), userBasePath, params), nil

}

func (r *Asset) createURL(userBasePath string) (string, error) {
	nr := r.urlNormalized()
	params := map[string]interface{}{
		"project":      dcl.ValueOrEmptyString(nr.Project),
		"location":     dcl.ValueOrEmptyString(nr.Location),
		"dataplexZone": dcl.ValueOrEmptyString(nr.DataplexZone),
		"lake":         dcl.ValueOrEmptyString(nr.Lake),
		"name":         dcl.ValueOrEmptyString(nr.Name),
	}
	return dcl.URL("projects/{{project}}/locations/{{location}}/lakes/{{lake}}/zones/{{dataplexZone}}/assets?assetId={{name}}", nr.basePath(), userBasePath, params), nil

}

func (r *Asset) deleteURL(userBasePath string) (string, error) {
	nr := r.urlNormalized()
	params := map[string]interface{}{
		"project":      dcl.ValueOrEmptyString(nr.Project),
		"location":     dcl.ValueOrEmptyString(nr.Location),
		"dataplexZone": dcl.ValueOrEmptyString(nr.DataplexZone),
		"lake":         dcl.ValueOrEmptyString(nr.Lake),
		"name":         dcl.ValueOrEmptyString(nr.Name),
	}
	return dcl.URL("projects/{{project}}/locations/{{location}}/lakes/{{lake}}/zones/{{dataplexZone}}/assets/{{name}}", nr.basePath(), userBasePath, params), nil
}

func (r *Asset) SetPolicyURL(userBasePath string) string {
	nr := r.urlNormalized()
	fields := map[string]interface{}{}
	return dcl.URL("", nr.basePath(), userBasePath, fields)
}

func (r *Asset) SetPolicyVerb() string {
	return ""
}

func (r *Asset) getPolicyURL(userBasePath string) string {
	nr := r.urlNormalized()
	fields := map[string]interface{}{}
	return dcl.URL("", nr.basePath(), userBasePath, fields)
}

func (r *Asset) IAMPolicyVersion() int {
	return 3
}

// assetApiOperation represents a mutable operation in the underlying REST
// API such as Create, Update, or Delete.
type assetApiOperation interface {
	do(context.Context, *Asset, *Client) error
}

// newUpdateAssetUpdateAssetRequest creates a request for an
// Asset resource's UpdateAsset update type by filling in the update
// fields based on the intended state of the resource.
func newUpdateAssetUpdateAssetRequest(ctx context.Context, f *Asset, c *Client) (map[string]interface{}, error) {
	req := map[string]interface{}{}
	res := f
	_ = res

	if v, err := dcl.DeriveField("projects/%s/locations/%s/lakes/%s/zones/%s/assets/%s", f.Name, dcl.SelfLinkToName(f.Project), dcl.SelfLinkToName(f.Location), dcl.SelfLinkToName(f.Lake), dcl.SelfLinkToName(f.DataplexZone), dcl.SelfLinkToName(f.Name)); err != nil {
		return nil, fmt.Errorf("error expanding Name into name: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		req["name"] = v
	}
	if v := f.DisplayName; !dcl.IsEmptyValueIndirect(v) {
		req["displayName"] = v
	}
	if v := f.Labels; !dcl.IsEmptyValueIndirect(v) {
		req["labels"] = v
	}
	if v := f.Description; !dcl.IsEmptyValueIndirect(v) {
		req["description"] = v
	}
	if v, err := expandAssetResourceSpec(c, f.ResourceSpec, res); err != nil {
		return nil, fmt.Errorf("error expanding ResourceSpec into resourceSpec: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		req["resourceSpec"] = v
	}
	if v, err := expandAssetResourceStatus(c, f.ResourceStatus, res); err != nil {
		return nil, fmt.Errorf("error expanding ResourceStatus into resourceStatus: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		req["resourceStatus"] = v
	}
	if v, err := expandAssetSecurityStatus(c, f.SecurityStatus, res); err != nil {
		return nil, fmt.Errorf("error expanding SecurityStatus into securityStatus: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		req["securityStatus"] = v
	}
	if v, err := expandAssetDiscoverySpec(c, f.DiscoverySpec, res); err != nil {
		return nil, fmt.Errorf("error expanding DiscoverySpec into discoverySpec: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		req["discoverySpec"] = v
	}
	if v, err := expandAssetDiscoveryStatus(c, f.DiscoveryStatus, res); err != nil {
		return nil, fmt.Errorf("error expanding DiscoveryStatus into discoveryStatus: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		req["discoveryStatus"] = v
	}
	req["name"] = fmt.Sprintf("projects/%s/locations/%s/lakes/%s/zones/%s/assets/%s", *f.Project, *f.Location, *f.Lake, *f.DataplexZone, *f.Name)

	return req, nil
}

// marshalUpdateAssetUpdateAssetRequest converts the update into
// the final JSON request body.
func marshalUpdateAssetUpdateAssetRequest(c *Client, m map[string]interface{}) ([]byte, error) {

	return json.Marshal(m)
}

type updateAssetUpdateAssetOperation struct {
	// If the update operation has the REQUIRES_APPLY_OPTIONS trait, this will be populated.
	// Usually it will be nil - this is to prevent us from accidentally depending on apply
	// options, which should usually be unnecessary.
	ApplyOptions []dcl.ApplyOption
	FieldDiffs   []*dcl.FieldDiff
}

// do creates a request and sends it to the appropriate URL. In most operations,
// do will transcribe a subset of the resource into a request object and send a
// PUT request to a single URL.

func (op *updateAssetUpdateAssetOperation) do(ctx context.Context, r *Asset, c *Client) error {
	_, err := c.GetAsset(ctx, r)
	if err != nil {
		return err
	}

	u, err := r.updateURL(c.Config.BasePath, "UpdateAsset")
	if err != nil {
		return err
	}
	mask := dcl.UpdateMask(op.FieldDiffs)
	u, err = dcl.AddQueryParams(u, map[string]string{"updateMask": mask})
	if err != nil {
		return err
	}

	req, err := newUpdateAssetUpdateAssetRequest(ctx, r, c)
	if err != nil {
		return err
	}

	c.Config.Logger.InfoWithContextf(ctx, "Created update: %#v", req)
	body, err := marshalUpdateAssetUpdateAssetRequest(c, req)
	if err != nil {
		return err
	}
	resp, err := dcl.SendRequest(ctx, c.Config, "PATCH", u, bytes.NewBuffer(body), c.Config.RetryProvider)
	if err != nil {
		return err
	}

	var o operations.StandardGCPOperation
	if err := dcl.ParseResponse(resp.Response, &o); err != nil {
		return err
	}
	err = o.Wait(context.WithValue(ctx, dcl.DoNotLogRequestsKey, true), c.Config, r.basePath(), "GET")

	if err != nil {
		return err
	}

	return nil
}

func (c *Client) listAssetRaw(ctx context.Context, r *Asset, pageToken string, pageSize int32) ([]byte, error) {
	u, err := r.urlNormalized().listURL(c.Config.BasePath)
	if err != nil {
		return nil, err
	}

	m := make(map[string]string)
	if pageToken != "" {
		m["pageToken"] = pageToken
	}

	if pageSize != AssetMaxPage {
		m["pageSize"] = fmt.Sprintf("%v", pageSize)
	}

	u, err = dcl.AddQueryParams(u, m)
	if err != nil {
		return nil, err
	}
	resp, err := dcl.SendRequest(ctx, c.Config, "GET", u, &bytes.Buffer{}, c.Config.RetryProvider)
	if err != nil {
		return nil, err
	}
	defer resp.Response.Body.Close()
	return ioutil.ReadAll(resp.Response.Body)
}

type listAssetOperation struct {
	Assets []map[string]interface{} `json:"assets"`
	Token  string                   `json:"nextPageToken"`
}

func (c *Client) listAsset(ctx context.Context, r *Asset, pageToken string, pageSize int32) ([]*Asset, string, error) {
	b, err := c.listAssetRaw(ctx, r, pageToken, pageSize)
	if err != nil {
		return nil, "", err
	}

	var m listAssetOperation
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, "", err
	}

	var l []*Asset
	for _, v := range m.Assets {
		res, err := unmarshalMapAsset(v, c, r)
		if err != nil {
			return nil, m.Token, err
		}
		res.Project = r.Project
		res.Location = r.Location
		res.DataplexZone = r.DataplexZone
		res.Lake = r.Lake
		l = append(l, res)
	}

	return l, m.Token, nil
}

func (c *Client) deleteAllAsset(ctx context.Context, f func(*Asset) bool, resources []*Asset) error {
	var errors []string
	for _, res := range resources {
		if f(res) {
			// We do not want deleteAll to fail on a deletion or else it will stop deleting other resources.
			err := c.DeleteAsset(ctx, res)
			if err != nil {
				errors = append(errors, err.Error())
			}
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("%v", strings.Join(errors, "\n"))
	} else {
		return nil
	}
}

type deleteAssetOperation struct{}

func (op *deleteAssetOperation) do(ctx context.Context, r *Asset, c *Client) error {
	r, err := c.GetAsset(ctx, r)
	if err != nil {
		if dcl.IsNotFound(err) {
			c.Config.Logger.InfoWithContextf(ctx, "Asset not found, returning. Original error: %v", err)
			return nil
		}
		c.Config.Logger.WarningWithContextf(ctx, "GetAsset checking for existence. error: %v", err)
		return err
	}

	u, err := r.deleteURL(c.Config.BasePath)
	if err != nil {
		return err
	}

	// Delete should never have a body
	body := &bytes.Buffer{}
	resp, err := dcl.SendRequest(ctx, c.Config, "DELETE", u, body, c.Config.RetryProvider)
	if err != nil {
		return err
	}

	// wait for object to be deleted.
	var o operations.StandardGCPOperation
	if err := dcl.ParseResponse(resp.Response, &o); err != nil {
		return err
	}
	if err := o.Wait(context.WithValue(ctx, dcl.DoNotLogRequestsKey, true), c.Config, r.basePath(), "GET"); err != nil {
		return err
	}

	// We saw a race condition where for some successful delete operation, the Get calls returned resources for a short duration.
	// This is the reason we are adding retry to handle that case.
	retriesRemaining := 10
	dcl.Do(ctx, func(ctx context.Context) (*dcl.RetryDetails, error) {
		_, err := c.GetAsset(ctx, r)
		if dcl.IsNotFound(err) {
			return nil, nil
		}
		if retriesRemaining > 0 {
			retriesRemaining--
			return &dcl.RetryDetails{}, dcl.OperationNotDone{}
		}
		return nil, dcl.NotDeletedError{ExistingResource: r}
	}, c.Config.RetryProvider)
	return nil
}

// Create operations are similar to Update operations, although they do not have
// specific request objects. The Create request object is the json encoding of
// the resource, which is modified by res.marshal to form the base request body.
type createAssetOperation struct {
	response map[string]interface{}
}

func (op *createAssetOperation) FirstResponse() (map[string]interface{}, bool) {
	return op.response, len(op.response) > 0
}

func (op *createAssetOperation) do(ctx context.Context, r *Asset, c *Client) error {
	c.Config.Logger.InfoWithContextf(ctx, "Attempting to create %v", r)
	u, err := r.createURL(c.Config.BasePath)
	if err != nil {
		return err
	}

	req, err := r.marshal(c)
	if err != nil {
		return err
	}
	resp, err := dcl.SendRequest(ctx, c.Config, "POST", u, bytes.NewBuffer(req), c.Config.RetryProvider)
	if err != nil {
		return err
	}
	// wait for object to be created.
	var o operations.StandardGCPOperation
	if err := dcl.ParseResponse(resp.Response, &o); err != nil {
		return err
	}
	if err := o.Wait(context.WithValue(ctx, dcl.DoNotLogRequestsKey, true), c.Config, r.basePath(), "GET"); err != nil {
		c.Config.Logger.Warningf("Creation failed after waiting for operation: %v", err)
		return err
	}
	c.Config.Logger.InfoWithContextf(ctx, "Successfully waited for operation")
	op.response, _ = o.FirstResponse()

	if _, err := c.GetAsset(ctx, r); err != nil {
		c.Config.Logger.WarningWithContextf(ctx, "get returned error: %v", err)
		return err
	}

	return nil
}

func (c *Client) getAssetRaw(ctx context.Context, r *Asset) ([]byte, error) {

	u, err := r.getURL(c.Config.BasePath)
	if err != nil {
		return nil, err
	}
	resp, err := dcl.SendRequest(ctx, c.Config, "GET", u, &bytes.Buffer{}, c.Config.RetryProvider)
	if err != nil {
		return nil, err
	}
	defer resp.Response.Body.Close()
	b, err := ioutil.ReadAll(resp.Response.Body)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (c *Client) assetDiffsForRawDesired(ctx context.Context, rawDesired *Asset, opts ...dcl.ApplyOption) (initial, desired *Asset, diffs []*dcl.FieldDiff, err error) {
	c.Config.Logger.InfoWithContext(ctx, "Fetching initial state...")
	// First, let us see if the user provided a state hint.  If they did, we will start fetching based on that.
	var fetchState *Asset
	if sh := dcl.FetchStateHint(opts); sh != nil {
		if r, ok := sh.(*Asset); !ok {
			c.Config.Logger.WarningWithContextf(ctx, "Initial state hint was of the wrong type; expected Asset, got %T", sh)
		} else {
			fetchState = r
		}
	}
	if fetchState == nil {
		fetchState = rawDesired
	}

	// 1.2: Retrieval of raw initial state from API
	rawInitial, err := c.GetAsset(ctx, fetchState)
	if rawInitial == nil {
		if !dcl.IsNotFound(err) {
			c.Config.Logger.WarningWithContextf(ctx, "Failed to retrieve whether a Asset resource already exists: %s", err)
			return nil, nil, nil, fmt.Errorf("failed to retrieve Asset resource: %v", err)
		}
		c.Config.Logger.InfoWithContext(ctx, "Found that Asset resource did not exist.")
		// Perform canonicalization to pick up defaults.
		desired, err = canonicalizeAssetDesiredState(rawDesired, rawInitial)
		return nil, desired, nil, err
	}
	c.Config.Logger.InfoWithContextf(ctx, "Found initial state for Asset: %v", rawInitial)
	c.Config.Logger.InfoWithContextf(ctx, "Initial desired state for Asset: %v", rawDesired)

	// The Get call applies postReadExtract and so the result may contain fields that are not part of API version.
	if err := extractAssetFields(rawInitial); err != nil {
		return nil, nil, nil, err
	}

	// 1.3: Canonicalize raw initial state into initial state.
	initial, err = canonicalizeAssetInitialState(rawInitial, rawDesired)
	if err != nil {
		return nil, nil, nil, err
	}
	c.Config.Logger.InfoWithContextf(ctx, "Canonicalized initial state for Asset: %v", initial)

	// 1.4: Canonicalize raw desired state into desired state.
	desired, err = canonicalizeAssetDesiredState(rawDesired, rawInitial, opts...)
	if err != nil {
		return nil, nil, nil, err
	}
	c.Config.Logger.InfoWithContextf(ctx, "Canonicalized desired state for Asset: %v", desired)

	// 2.1: Comparison of initial and desired state.
	diffs, err = diffAsset(c, desired, initial, opts...)
	return initial, desired, diffs, err
}

func canonicalizeAssetInitialState(rawInitial, rawDesired *Asset) (*Asset, error) {
	// TODO(magic-modules-eng): write canonicalizer once relevant traits are added.
	return rawInitial, nil
}

/*
* Canonicalizers
*
* These are responsible for converting either a user-specified config or a
* GCP API response to a standard format that can be used for difference checking.
* */

func canonicalizeAssetDesiredState(rawDesired, rawInitial *Asset, opts ...dcl.ApplyOption) (*Asset, error) {

	if rawInitial == nil {
		// Since the initial state is empty, the desired state is all we have.
		// We canonicalize the remaining nested objects with nil to pick up defaults.
		rawDesired.ResourceSpec = canonicalizeAssetResourceSpec(rawDesired.ResourceSpec, nil, opts...)
		rawDesired.ResourceStatus = canonicalizeAssetResourceStatus(rawDesired.ResourceStatus, nil, opts...)
		rawDesired.SecurityStatus = canonicalizeAssetSecurityStatus(rawDesired.SecurityStatus, nil, opts...)
		rawDesired.DiscoverySpec = canonicalizeAssetDiscoverySpec(rawDesired.DiscoverySpec, nil, opts...)
		rawDesired.DiscoveryStatus = canonicalizeAssetDiscoveryStatus(rawDesired.DiscoveryStatus, nil, opts...)

		return rawDesired, nil
	}
	canonicalDesired := &Asset{}
	if dcl.PartialSelfLinkToSelfLink(rawDesired.Name, rawInitial.Name) {
		canonicalDesired.Name = rawInitial.Name
	} else {
		canonicalDesired.Name = rawDesired.Name
	}
	if dcl.StringCanonicalize(rawDesired.DisplayName, rawInitial.DisplayName) {
		canonicalDesired.DisplayName = rawInitial.DisplayName
	} else {
		canonicalDesired.DisplayName = rawDesired.DisplayName
	}
	if dcl.IsZeroValue(rawDesired.Labels) || (dcl.IsEmptyValueIndirect(rawDesired.Labels) && dcl.IsEmptyValueIndirect(rawInitial.Labels)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		canonicalDesired.Labels = rawInitial.Labels
	} else {
		canonicalDesired.Labels = rawDesired.Labels
	}
	if dcl.StringCanonicalize(rawDesired.Description, rawInitial.Description) {
		canonicalDesired.Description = rawInitial.Description
	} else {
		canonicalDesired.Description = rawDesired.Description
	}
	canonicalDesired.ResourceSpec = canonicalizeAssetResourceSpec(rawDesired.ResourceSpec, rawInitial.ResourceSpec, opts...)
	canonicalDesired.DiscoverySpec = canonicalizeAssetDiscoverySpec(rawDesired.DiscoverySpec, rawInitial.DiscoverySpec, opts...)
	if dcl.NameToSelfLink(rawDesired.Project, rawInitial.Project) {
		canonicalDesired.Project = rawInitial.Project
	} else {
		canonicalDesired.Project = rawDesired.Project
	}
	if dcl.NameToSelfLink(rawDesired.Location, rawInitial.Location) {
		canonicalDesired.Location = rawInitial.Location
	} else {
		canonicalDesired.Location = rawDesired.Location
	}
	if dcl.NameToSelfLink(rawDesired.Lake, rawInitial.Lake) {
		canonicalDesired.Lake = rawInitial.Lake
	} else {
		canonicalDesired.Lake = rawDesired.Lake
	}
	if dcl.NameToSelfLink(rawDesired.DataplexZone, rawInitial.DataplexZone) {
		canonicalDesired.DataplexZone = rawInitial.DataplexZone
	} else {
		canonicalDesired.DataplexZone = rawDesired.DataplexZone
	}
	return canonicalDesired, nil
}

func canonicalizeAssetNewState(c *Client, rawNew, rawDesired *Asset) (*Asset, error) {

	if dcl.IsEmptyValueIndirect(rawNew.Name) && dcl.IsEmptyValueIndirect(rawDesired.Name) {
		rawNew.Name = rawDesired.Name
	} else {
		if dcl.PartialSelfLinkToSelfLink(rawDesired.Name, rawNew.Name) {
			rawNew.Name = rawDesired.Name
		}
	}

	if dcl.IsEmptyValueIndirect(rawNew.DisplayName) && dcl.IsEmptyValueIndirect(rawDesired.DisplayName) {
		rawNew.DisplayName = rawDesired.DisplayName
	} else {
		if dcl.StringCanonicalize(rawDesired.DisplayName, rawNew.DisplayName) {
			rawNew.DisplayName = rawDesired.DisplayName
		}
	}

	if dcl.IsEmptyValueIndirect(rawNew.Uid) && dcl.IsEmptyValueIndirect(rawDesired.Uid) {
		rawNew.Uid = rawDesired.Uid
	} else {
		if dcl.StringCanonicalize(rawDesired.Uid, rawNew.Uid) {
			rawNew.Uid = rawDesired.Uid
		}
	}

	if dcl.IsEmptyValueIndirect(rawNew.CreateTime) && dcl.IsEmptyValueIndirect(rawDesired.CreateTime) {
		rawNew.CreateTime = rawDesired.CreateTime
	} else {
	}

	if dcl.IsEmptyValueIndirect(rawNew.UpdateTime) && dcl.IsEmptyValueIndirect(rawDesired.UpdateTime) {
		rawNew.UpdateTime = rawDesired.UpdateTime
	} else {
	}

	if dcl.IsEmptyValueIndirect(rawNew.Labels) && dcl.IsEmptyValueIndirect(rawDesired.Labels) {
		rawNew.Labels = rawDesired.Labels
	} else {
	}

	if dcl.IsEmptyValueIndirect(rawNew.Description) && dcl.IsEmptyValueIndirect(rawDesired.Description) {
		rawNew.Description = rawDesired.Description
	} else {
		if dcl.StringCanonicalize(rawDesired.Description, rawNew.Description) {
			rawNew.Description = rawDesired.Description
		}
	}

	if dcl.IsEmptyValueIndirect(rawNew.State) && dcl.IsEmptyValueIndirect(rawDesired.State) {
		rawNew.State = rawDesired.State
	} else {
	}

	if dcl.IsEmptyValueIndirect(rawNew.ResourceSpec) && dcl.IsEmptyValueIndirect(rawDesired.ResourceSpec) {
		rawNew.ResourceSpec = rawDesired.ResourceSpec
	} else {
		rawNew.ResourceSpec = canonicalizeNewAssetResourceSpec(c, rawDesired.ResourceSpec, rawNew.ResourceSpec)
	}

	if dcl.IsEmptyValueIndirect(rawNew.ResourceStatus) && dcl.IsEmptyValueIndirect(rawDesired.ResourceStatus) {
		rawNew.ResourceStatus = rawDesired.ResourceStatus
	} else {
		rawNew.ResourceStatus = canonicalizeNewAssetResourceStatus(c, rawDesired.ResourceStatus, rawNew.ResourceStatus)
	}

	if dcl.IsEmptyValueIndirect(rawNew.SecurityStatus) && dcl.IsEmptyValueIndirect(rawDesired.SecurityStatus) {
		rawNew.SecurityStatus = rawDesired.SecurityStatus
	} else {
		rawNew.SecurityStatus = canonicalizeNewAssetSecurityStatus(c, rawDesired.SecurityStatus, rawNew.SecurityStatus)
	}

	if dcl.IsEmptyValueIndirect(rawNew.DiscoverySpec) && dcl.IsEmptyValueIndirect(rawDesired.DiscoverySpec) {
		rawNew.DiscoverySpec = rawDesired.DiscoverySpec
	} else {
		rawNew.DiscoverySpec = canonicalizeNewAssetDiscoverySpec(c, rawDesired.DiscoverySpec, rawNew.DiscoverySpec)
	}

	if dcl.IsEmptyValueIndirect(rawNew.DiscoveryStatus) && dcl.IsEmptyValueIndirect(rawDesired.DiscoveryStatus) {
		rawNew.DiscoveryStatus = rawDesired.DiscoveryStatus
	} else {
		rawNew.DiscoveryStatus = canonicalizeNewAssetDiscoveryStatus(c, rawDesired.DiscoveryStatus, rawNew.DiscoveryStatus)
	}

	rawNew.Project = rawDesired.Project

	rawNew.Location = rawDesired.Location

	rawNew.Lake = rawDesired.Lake

	rawNew.DataplexZone = rawDesired.DataplexZone

	return rawNew, nil
}

func canonicalizeAssetResourceSpec(des, initial *AssetResourceSpec, opts ...dcl.ApplyOption) *AssetResourceSpec {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetResourceSpec{}

	if dcl.StringCanonicalize(des.Name, initial.Name) || dcl.IsZeroValue(des.Name) {
		cDes.Name = initial.Name
	} else {
		cDes.Name = des.Name
	}
	if dcl.IsZeroValue(des.Type) || (dcl.IsEmptyValueIndirect(des.Type) && dcl.IsEmptyValueIndirect(initial.Type)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.Type = initial.Type
	} else {
		cDes.Type = des.Type
	}
	if dcl.IsZeroValue(des.ReadAccessMode) || (dcl.IsEmptyValueIndirect(des.ReadAccessMode) && dcl.IsEmptyValueIndirect(initial.ReadAccessMode)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.ReadAccessMode = initial.ReadAccessMode
	} else {
		cDes.ReadAccessMode = des.ReadAccessMode
	}

	return cDes
}

func canonicalizeNewAssetResourceSpec(c *Client, des, nw *AssetResourceSpec) *AssetResourceSpec {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetResourceSpec while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.StringCanonicalize(des.Name, nw.Name) {
		nw.Name = des.Name
	}

	return nw
}

func canonicalizeAssetResourceStatus(des, initial *AssetResourceStatus, opts ...dcl.ApplyOption) *AssetResourceStatus {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetResourceStatus{}

	if dcl.IsZeroValue(des.State) || (dcl.IsEmptyValueIndirect(des.State) && dcl.IsEmptyValueIndirect(initial.State)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.State = initial.State
	} else {
		cDes.State = des.State
	}
	if dcl.StringCanonicalize(des.Message, initial.Message) || dcl.IsZeroValue(des.Message) {
		cDes.Message = initial.Message
	} else {
		cDes.Message = des.Message
	}
	if dcl.IsZeroValue(des.UpdateTime) || (dcl.IsEmptyValueIndirect(des.UpdateTime) && dcl.IsEmptyValueIndirect(initial.UpdateTime)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.UpdateTime = initial.UpdateTime
	} else {
		cDes.UpdateTime = des.UpdateTime
	}

	return cDes
}

func canonicalizeNewAssetResourceStatus(c *Client, des, nw *AssetResourceStatus) *AssetResourceStatus {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetResourceStatus while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.StringCanonicalize(des.Message, nw.Message) {
		nw.Message = des.Message
	}

	return nw
}

func canonicalizeAssetSecurityStatus(des, initial *AssetSecurityStatus, opts ...dcl.ApplyOption) *AssetSecurityStatus {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetSecurityStatus{}

	if dcl.IsZeroValue(des.State) || (dcl.IsEmptyValueIndirect(des.State) && dcl.IsEmptyValueIndirect(initial.State)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.State = initial.State
	} else {
		cDes.State = des.State
	}
	if dcl.StringCanonicalize(des.Message, initial.Message) || dcl.IsZeroValue(des.Message) {
		cDes.Message = initial.Message
	} else {
		cDes.Message = des.Message
	}
	if dcl.IsZeroValue(des.UpdateTime) || (dcl.IsEmptyValueIndirect(des.UpdateTime) && dcl.IsEmptyValueIndirect(initial.UpdateTime)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.UpdateTime = initial.UpdateTime
	} else {
		cDes.UpdateTime = des.UpdateTime
	}

	return cDes
}

func canonicalizeNewAssetSecurityStatus(c *Client, des, nw *AssetSecurityStatus) *AssetSecurityStatus {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetSecurityStatus while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.StringCanonicalize(des.Message, nw.Message) {
		nw.Message = des.Message
	}

	return nw
}

func canonicalizeAssetDiscoverySpec(des, initial *AssetDiscoverySpec, opts ...dcl.ApplyOption) *AssetDiscoverySpec {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetDiscoverySpec{}

	if dcl.BoolCanonicalize(des.Enabled, initial.Enabled) || dcl.IsZeroValue(des.Enabled) {
		cDes.Enabled = initial.Enabled
	} else {
		cDes.Enabled = des.Enabled
	}
	if dcl.StringArrayCanonicalize(des.IncludePatterns, initial.IncludePatterns) {
		cDes.IncludePatterns = initial.IncludePatterns
	} else {
		cDes.IncludePatterns = des.IncludePatterns
	}
	if dcl.StringArrayCanonicalize(des.ExcludePatterns, initial.ExcludePatterns) {
		cDes.ExcludePatterns = initial.ExcludePatterns
	} else {
		cDes.ExcludePatterns = des.ExcludePatterns
	}
	cDes.CsvOptions = canonicalizeAssetDiscoverySpecCsvOptions(des.CsvOptions, initial.CsvOptions, opts...)
	cDes.JsonOptions = canonicalizeAssetDiscoverySpecJsonOptions(des.JsonOptions, initial.JsonOptions, opts...)
	if dcl.StringCanonicalize(des.Schedule, initial.Schedule) || dcl.IsZeroValue(des.Schedule) {
		cDes.Schedule = initial.Schedule
	} else {
		cDes.Schedule = des.Schedule
	}

	return cDes
}

func canonicalizeNewAssetDiscoverySpec(c *Client, des, nw *AssetDiscoverySpec) *AssetDiscoverySpec {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetDiscoverySpec while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.BoolCanonicalize(des.Enabled, nw.Enabled) {
		nw.Enabled = des.Enabled
	}
	if dcl.StringArrayCanonicalize(des.IncludePatterns, nw.IncludePatterns) {
		nw.IncludePatterns = des.IncludePatterns
	}
	if dcl.StringArrayCanonicalize(des.ExcludePatterns, nw.ExcludePatterns) {
		nw.ExcludePatterns = des.ExcludePatterns
	}
	nw.CsvOptions = canonicalizeNewAssetDiscoverySpecCsvOptions(c, des.CsvOptions, nw.CsvOptions)
	nw.JsonOptions = canonicalizeNewAssetDiscoverySpecJsonOptions(c, des.JsonOptions, nw.JsonOptions)
	if dcl.StringCanonicalize(des.Schedule, nw.Schedule) {
		nw.Schedule = des.Schedule
	}

	return nw
}

func canonicalizeAssetDiscoverySpecCsvOptions(des, initial *AssetDiscoverySpecCsvOptions, opts ...dcl.ApplyOption) *AssetDiscoverySpecCsvOptions {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetDiscoverySpecCsvOptions{}

	if dcl.IsZeroValue(des.HeaderRows) || (dcl.IsEmptyValueIndirect(des.HeaderRows) && dcl.IsEmptyValueIndirect(initial.HeaderRows)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.HeaderRows = initial.HeaderRows
	} else {
		cDes.HeaderRows = des.HeaderRows
	}
	if dcl.StringCanonicalize(des.Delimiter, initial.Delimiter) || dcl.IsZeroValue(des.Delimiter) {
		cDes.Delimiter = initial.Delimiter
	} else {
		cDes.Delimiter = des.Delimiter
	}
	if dcl.StringCanonicalize(des.Encoding, initial.Encoding) || dcl.IsZeroValue(des.Encoding) {
		cDes.Encoding = initial.Encoding
	} else {
		cDes.Encoding = des.Encoding
	}
	if dcl.BoolCanonicalize(des.DisableTypeInference, initial.DisableTypeInference) || dcl.IsZeroValue(des.DisableTypeInference) {
		cDes.DisableTypeInference = initial.DisableTypeInference
	} else {
		cDes.DisableTypeInference = des.DisableTypeInference
	}

	return cDes
}

func canonicalizeNewAssetDiscoverySpecCsvOptions(c *Client, des, nw *AssetDiscoverySpecCsvOptions) *AssetDiscoverySpecCsvOptions {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetDiscoverySpecCsvOptions while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.StringCanonicalize(des.Delimiter, nw.Delimiter) {
		nw.Delimiter = des.Delimiter
	}
	if dcl.StringCanonicalize(des.Encoding, nw.Encoding) {
		nw.Encoding = des.Encoding
	}
	if dcl.BoolCanonicalize(des.DisableTypeInference, nw.DisableTypeInference) {
		nw.DisableTypeInference = des.DisableTypeInference
	}

	return nw
}

func canonicalizeAssetDiscoverySpecJsonOptions(des, initial *AssetDiscoverySpecJsonOptions, opts ...dcl.ApplyOption) *AssetDiscoverySpecJsonOptions {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetDiscoverySpecJsonOptions{}

	if dcl.StringCanonicalize(des.Encoding, initial.Encoding) || dcl.IsZeroValue(des.Encoding) {
		cDes.Encoding = initial.Encoding
	} else {
		cDes.Encoding = des.Encoding
	}
	if dcl.BoolCanonicalize(des.DisableTypeInference, initial.DisableTypeInference) || dcl.IsZeroValue(des.DisableTypeInference) {
		cDes.DisableTypeInference = initial.DisableTypeInference
	} else {
		cDes.DisableTypeInference = des.DisableTypeInference
	}

	return cDes
}

func canonicalizeNewAssetDiscoverySpecJsonOptions(c *Client, des, nw *AssetDiscoverySpecJsonOptions) *AssetDiscoverySpecJsonOptions {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetDiscoverySpecJsonOptions while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.StringCanonicalize(des.Encoding, nw.Encoding) {
		nw.Encoding = des.Encoding
	}
	if dcl.BoolCanonicalize(des.DisableTypeInference, nw.DisableTypeInference) {
		nw.DisableTypeInference = des.DisableTypeInference
	}

	return nw
}

func canonicalizeAssetDiscoveryStatus(des, initial *AssetDiscoveryStatus, opts ...dcl.ApplyOption) *AssetDiscoveryStatus {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetDiscoveryStatus{}

	if dcl.IsZeroValue(des.State) || (dcl.IsEmptyValueIndirect(des.State) && dcl.IsEmptyValueIndirect(initial.State)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.State = initial.State
	} else {
		cDes.State = des.State
	}
	if dcl.StringCanonicalize(des.Message, initial.Message) || dcl.IsZeroValue(des.Message) {
		cDes.Message = initial.Message
	} else {
		cDes.Message = des.Message
	}
	if dcl.IsZeroValue(des.UpdateTime) || (dcl.IsEmptyValueIndirect(des.UpdateTime) && dcl.IsEmptyValueIndirect(initial.UpdateTime)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.UpdateTime = initial.UpdateTime
	} else {
		cDes.UpdateTime = des.UpdateTime
	}
	if dcl.IsZeroValue(des.LastRunTime) || (dcl.IsEmptyValueIndirect(des.LastRunTime) && dcl.IsEmptyValueIndirect(initial.LastRunTime)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.LastRunTime = initial.LastRunTime
	} else {
		cDes.LastRunTime = des.LastRunTime
	}
	cDes.Stats = canonicalizeAssetDiscoveryStatusStats(des.Stats, initial.Stats, opts...)
	if dcl.StringCanonicalize(des.LastRunDuration, initial.LastRunDuration) || dcl.IsZeroValue(des.LastRunDuration) {
		cDes.LastRunDuration = initial.LastRunDuration
	} else {
		cDes.LastRunDuration = des.LastRunDuration
	}

	return cDes
}

func canonicalizeNewAssetDiscoveryStatus(c *Client, des, nw *AssetDiscoveryStatus) *AssetDiscoveryStatus {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetDiscoveryStatus while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	if dcl.StringCanonicalize(des.Message, nw.Message) {
		nw.Message = des.Message
	}
	nw.Stats = canonicalizeNewAssetDiscoveryStatusStats(c, des.Stats, nw.Stats)
	if dcl.StringCanonicalize(des.LastRunDuration, nw.LastRunDuration) {
		nw.LastRunDuration = des.LastRunDuration
	}

	return nw
}

func canonicalizeAssetDiscoveryStatusStats(des, initial *AssetDiscoveryStatusStats, opts ...dcl.ApplyOption) *AssetDiscoveryStatusStats {
	if des == nil {
		return initial
	}
	if des.empty {
		return des
	}

	if initial == nil {
		return des
	}

	cDes := &AssetDiscoveryStatusStats{}

	if dcl.IsZeroValue(des.DataItems) || (dcl.IsEmptyValueIndirect(des.DataItems) && dcl.IsEmptyValueIndirect(initial.DataItems)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.DataItems = initial.DataItems
	} else {
		cDes.DataItems = des.DataItems
	}
	if dcl.IsZeroValue(des.DataSize) || (dcl.IsEmptyValueIndirect(des.DataSize) && dcl.IsEmptyValueIndirect(initial.DataSize)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.DataSize = initial.DataSize
	} else {
		cDes.DataSize = des.DataSize
	}
	if dcl.IsZeroValue(des.Tables) || (dcl.IsEmptyValueIndirect(des.Tables) && dcl.IsEmptyValueIndirect(initial.Tables)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.Tables = initial.Tables
	} else {
		cDes.Tables = des.Tables
	}
	if dcl.IsZeroValue(des.Filesets) || (dcl.IsEmptyValueIndirect(des.Filesets) && dcl.IsEmptyValueIndirect(initial.Filesets)) {
		// Desired and initial values are equivalent, so set canonical desired value to initial value.
		cDes.Filesets = initial.Filesets
	} else {
		cDes.Filesets = des.Filesets
	}

	return cDes
}

func canonicalizeNewAssetDiscoveryStatusStats(c *Client, des, nw *AssetDiscoveryStatusStats) *AssetDiscoveryStatusStats {

	if des == nil {
		return nw
	}

	if nw == nil {
		if dcl.IsEmptyValueIndirect(des) {
			c.Config.Logger.Info("Found explicitly empty value for AssetDiscoveryStatusStats while comparing non-nil desired to nil actual.  Returning desired object.")
			return des
		}
		return nil
	}

	return nw
}

// The differ returns a list of diffs, along with a list of operations that should be taken
// to remedy them. Right now, it does not attempt to consolidate operations - if several
// fields can be fixed with a patch update, it will perform the patch several times.
// Diffs on some fields will be ignored if the `desired` state has an empty (nil)
// value. This empty value indicates that the user does not care about the state for
// the field. Empty fields on the actual object will cause diffs.
// TODO(magic-modules-eng): for efficiency in some resources, add batching.
func diffAsset(c *Client, desired, actual *Asset, opts ...dcl.ApplyOption) ([]*dcl.FieldDiff, error) {
	if desired == nil || actual == nil {
		return nil, fmt.Errorf("nil resource passed to diff - always a programming error: %#v, %#v", desired, actual)
	}

	c.Config.Logger.Infof("Diff function called with desired state: %v", desired)
	c.Config.Logger.Infof("Diff function called with actual state: %v", actual)

	var fn dcl.FieldName
	var newDiffs []*dcl.FieldDiff
	// New style diffs.
	if ds, err := dcl.Diff(desired.Name, actual.Name, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Name")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DisplayName, actual.DisplayName, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("DisplayName")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Uid, actual.Uid, dcl.DiffInfo{OutputOnly: true, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Uid")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.CreateTime, actual.CreateTime, dcl.DiffInfo{OutputOnly: true, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("CreateTime")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.UpdateTime, actual.UpdateTime, dcl.DiffInfo{OutputOnly: true, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("UpdateTime")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Labels, actual.Labels, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Labels")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Description, actual.Description, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Description")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.State, actual.State, dcl.DiffInfo{OutputOnly: true, Type: "EnumType", OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("State")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.ResourceSpec, actual.ResourceSpec, dcl.DiffInfo{ObjectFunction: compareAssetResourceSpecNewStyle, EmptyObject: EmptyAssetResourceSpec, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("ResourceSpec")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.ResourceStatus, actual.ResourceStatus, dcl.DiffInfo{OutputOnly: true, ObjectFunction: compareAssetResourceStatusNewStyle, EmptyObject: EmptyAssetResourceStatus, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("ResourceStatus")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.SecurityStatus, actual.SecurityStatus, dcl.DiffInfo{OutputOnly: true, ObjectFunction: compareAssetSecurityStatusNewStyle, EmptyObject: EmptyAssetSecurityStatus, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("SecurityStatus")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DiscoverySpec, actual.DiscoverySpec, dcl.DiffInfo{ObjectFunction: compareAssetDiscoverySpecNewStyle, EmptyObject: EmptyAssetDiscoverySpec, OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("DiscoverySpec")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DiscoveryStatus, actual.DiscoveryStatus, dcl.DiffInfo{OutputOnly: true, ObjectFunction: compareAssetDiscoveryStatusNewStyle, EmptyObject: EmptyAssetDiscoveryStatus, OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("DiscoveryStatus")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Project, actual.Project, dcl.DiffInfo{Type: "ReferenceType", OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Project")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Location, actual.Location, dcl.DiffInfo{OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Location")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Lake, actual.Lake, dcl.DiffInfo{OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Lake")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DataplexZone, actual.DataplexZone, dcl.DiffInfo{OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Zone")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		newDiffs = append(newDiffs, ds...)
	}

	if len(newDiffs) > 0 {
		c.Config.Logger.Infof("Diff function found diffs: %v", newDiffs)
	}
	return newDiffs, nil
}
func compareAssetResourceSpecNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetResourceSpec)
	if !ok {
		desiredNotPointer, ok := d.(AssetResourceSpec)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetResourceSpec or *AssetResourceSpec", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetResourceSpec)
	if !ok {
		actualNotPointer, ok := a.(AssetResourceSpec)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetResourceSpec", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.Name, actual.Name, dcl.DiffInfo{OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Name")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Type, actual.Type, dcl.DiffInfo{Type: "EnumType", OperationSelector: dcl.RequiresRecreate()}, fn.AddNest("Type")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.ReadAccessMode, actual.ReadAccessMode, dcl.DiffInfo{ServerDefault: true, Type: "EnumType", OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("ReadAccessMode")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetResourceStatusNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetResourceStatus)
	if !ok {
		desiredNotPointer, ok := d.(AssetResourceStatus)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetResourceStatus or *AssetResourceStatus", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetResourceStatus)
	if !ok {
		actualNotPointer, ok := a.(AssetResourceStatus)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetResourceStatus", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.State, actual.State, dcl.DiffInfo{Type: "EnumType", OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("State")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Message, actual.Message, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Message")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.UpdateTime, actual.UpdateTime, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("UpdateTime")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetSecurityStatusNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetSecurityStatus)
	if !ok {
		desiredNotPointer, ok := d.(AssetSecurityStatus)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetSecurityStatus or *AssetSecurityStatus", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetSecurityStatus)
	if !ok {
		actualNotPointer, ok := a.(AssetSecurityStatus)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetSecurityStatus", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.State, actual.State, dcl.DiffInfo{Type: "EnumType", OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("State")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Message, actual.Message, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Message")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.UpdateTime, actual.UpdateTime, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("UpdateTime")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetDiscoverySpecNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetDiscoverySpec)
	if !ok {
		desiredNotPointer, ok := d.(AssetDiscoverySpec)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoverySpec or *AssetDiscoverySpec", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetDiscoverySpec)
	if !ok {
		actualNotPointer, ok := a.(AssetDiscoverySpec)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoverySpec", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.Enabled, actual.Enabled, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Enabled")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.IncludePatterns, actual.IncludePatterns, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("IncludePatterns")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.ExcludePatterns, actual.ExcludePatterns, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("ExcludePatterns")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.CsvOptions, actual.CsvOptions, dcl.DiffInfo{ServerDefault: true, ObjectFunction: compareAssetDiscoverySpecCsvOptionsNewStyle, EmptyObject: EmptyAssetDiscoverySpecCsvOptions, OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("CsvOptions")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.JsonOptions, actual.JsonOptions, dcl.DiffInfo{ServerDefault: true, ObjectFunction: compareAssetDiscoverySpecJsonOptionsNewStyle, EmptyObject: EmptyAssetDiscoverySpecJsonOptions, OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("JsonOptions")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Schedule, actual.Schedule, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Schedule")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetDiscoverySpecCsvOptionsNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetDiscoverySpecCsvOptions)
	if !ok {
		desiredNotPointer, ok := d.(AssetDiscoverySpecCsvOptions)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoverySpecCsvOptions or *AssetDiscoverySpecCsvOptions", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetDiscoverySpecCsvOptions)
	if !ok {
		actualNotPointer, ok := a.(AssetDiscoverySpecCsvOptions)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoverySpecCsvOptions", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.HeaderRows, actual.HeaderRows, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("HeaderRows")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Delimiter, actual.Delimiter, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Delimiter")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Encoding, actual.Encoding, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Encoding")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DisableTypeInference, actual.DisableTypeInference, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("DisableTypeInference")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetDiscoverySpecJsonOptionsNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetDiscoverySpecJsonOptions)
	if !ok {
		desiredNotPointer, ok := d.(AssetDiscoverySpecJsonOptions)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoverySpecJsonOptions or *AssetDiscoverySpecJsonOptions", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetDiscoverySpecJsonOptions)
	if !ok {
		actualNotPointer, ok := a.(AssetDiscoverySpecJsonOptions)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoverySpecJsonOptions", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.Encoding, actual.Encoding, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Encoding")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DisableTypeInference, actual.DisableTypeInference, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("DisableTypeInference")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetDiscoveryStatusNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetDiscoveryStatus)
	if !ok {
		desiredNotPointer, ok := d.(AssetDiscoveryStatus)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoveryStatus or *AssetDiscoveryStatus", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetDiscoveryStatus)
	if !ok {
		actualNotPointer, ok := a.(AssetDiscoveryStatus)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoveryStatus", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.State, actual.State, dcl.DiffInfo{Type: "EnumType", OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("State")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Message, actual.Message, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Message")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.UpdateTime, actual.UpdateTime, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("UpdateTime")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.LastRunTime, actual.LastRunTime, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("LastRunTime")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Stats, actual.Stats, dcl.DiffInfo{ObjectFunction: compareAssetDiscoveryStatusStatsNewStyle, EmptyObject: EmptyAssetDiscoveryStatusStats, OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Stats")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.LastRunDuration, actual.LastRunDuration, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("LastRunDuration")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

func compareAssetDiscoveryStatusStatsNewStyle(d, a interface{}, fn dcl.FieldName) ([]*dcl.FieldDiff, error) {
	var diffs []*dcl.FieldDiff

	desired, ok := d.(*AssetDiscoveryStatusStats)
	if !ok {
		desiredNotPointer, ok := d.(AssetDiscoveryStatusStats)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoveryStatusStats or *AssetDiscoveryStatusStats", d)
		}
		desired = &desiredNotPointer
	}
	actual, ok := a.(*AssetDiscoveryStatusStats)
	if !ok {
		actualNotPointer, ok := a.(AssetDiscoveryStatusStats)
		if !ok {
			return nil, fmt.Errorf("obj %v is not a AssetDiscoveryStatusStats", a)
		}
		actual = &actualNotPointer
	}

	if ds, err := dcl.Diff(desired.DataItems, actual.DataItems, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("DataItems")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.DataSize, actual.DataSize, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("DataSize")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Tables, actual.Tables, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Tables")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}

	if ds, err := dcl.Diff(desired.Filesets, actual.Filesets, dcl.DiffInfo{OperationSelector: dcl.TriggersOperation("updateAssetUpdateAssetOperation")}, fn.AddNest("Filesets")); len(ds) != 0 || err != nil {
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, ds...)
	}
	return diffs, nil
}

// urlNormalized returns a copy of the resource struct with values normalized
// for URL substitutions. For instance, it converts long-form self-links to
// short-form so they can be substituted in.
func (r *Asset) urlNormalized() *Asset {
	normalized := dcl.Copy(*r).(Asset)
	normalized.Name = dcl.SelfLinkToName(r.Name)
	normalized.DisplayName = dcl.SelfLinkToName(r.DisplayName)
	normalized.Uid = dcl.SelfLinkToName(r.Uid)
	normalized.Description = dcl.SelfLinkToName(r.Description)
	normalized.Project = dcl.SelfLinkToName(r.Project)
	normalized.Location = dcl.SelfLinkToName(r.Location)
	normalized.Lake = dcl.SelfLinkToName(r.Lake)
	normalized.DataplexZone = dcl.SelfLinkToName(r.DataplexZone)
	return &normalized
}

func (r *Asset) updateURL(userBasePath, updateName string) (string, error) {
	nr := r.urlNormalized()
	if updateName == "UpdateAsset" {
		fields := map[string]interface{}{
			"project":      dcl.ValueOrEmptyString(nr.Project),
			"location":     dcl.ValueOrEmptyString(nr.Location),
			"dataplexZone": dcl.ValueOrEmptyString(nr.DataplexZone),
			"lake":         dcl.ValueOrEmptyString(nr.Lake),
			"name":         dcl.ValueOrEmptyString(nr.Name),
		}
		return dcl.URL("projects/{{project}}/locations/{{location}}/lakes/{{lake}}/zones/{{dataplexZone}}/assets/{{name}}", nr.basePath(), userBasePath, fields), nil

	}

	return "", fmt.Errorf("unknown update name: %s", updateName)
}

// marshal encodes the Asset resource into JSON for a Create request, and
// performs transformations from the resource schema to the API schema if
// necessary.
func (r *Asset) marshal(c *Client) ([]byte, error) {
	m, err := expandAsset(c, r)
	if err != nil {
		return nil, fmt.Errorf("error marshalling Asset: %w", err)
	}

	return json.Marshal(m)
}

// unmarshalAsset decodes JSON responses into the Asset resource schema.
func unmarshalAsset(b []byte, c *Client, res *Asset) (*Asset, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return unmarshalMapAsset(m, c, res)
}

func unmarshalMapAsset(m map[string]interface{}, c *Client, res *Asset) (*Asset, error) {

	flattened := flattenAsset(c, m, res)
	if flattened == nil {
		return nil, fmt.Errorf("attempted to flatten empty json object")
	}
	return flattened, nil
}

// expandAsset expands Asset into a JSON request object.
func expandAsset(c *Client, f *Asset) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	res := f
	_ = res
	if v, err := dcl.DeriveField("projects/%s/locations/%s/lakes/%s/zones/%s/assets/%s", f.Name, dcl.SelfLinkToName(f.Project), dcl.SelfLinkToName(f.Location), dcl.SelfLinkToName(f.Lake), dcl.SelfLinkToName(f.DataplexZone), dcl.SelfLinkToName(f.Name)); err != nil {
		return nil, fmt.Errorf("error expanding Name into name: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["name"] = v
	}
	if v := f.DisplayName; dcl.ValueShouldBeSent(v) {
		m["displayName"] = v
	}
	if v := f.Labels; dcl.ValueShouldBeSent(v) {
		m["labels"] = v
	}
	if v := f.Description; dcl.ValueShouldBeSent(v) {
		m["description"] = v
	}
	if v, err := expandAssetResourceSpec(c, f.ResourceSpec, res); err != nil {
		return nil, fmt.Errorf("error expanding ResourceSpec into resourceSpec: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["resourceSpec"] = v
	}
	if v, err := expandAssetDiscoverySpec(c, f.DiscoverySpec, res); err != nil {
		return nil, fmt.Errorf("error expanding DiscoverySpec into discoverySpec: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["discoverySpec"] = v
	}
	if v, err := dcl.EmptyValue(); err != nil {
		return nil, fmt.Errorf("error expanding Project into project: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["project"] = v
	}
	if v, err := dcl.EmptyValue(); err != nil {
		return nil, fmt.Errorf("error expanding Location into location: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["location"] = v
	}
	if v, err := dcl.EmptyValue(); err != nil {
		return nil, fmt.Errorf("error expanding Lake into lake: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["lake"] = v
	}
	if v, err := dcl.EmptyValue(); err != nil {
		return nil, fmt.Errorf("error expanding DataplexZone into zone: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["zone"] = v
	}

	return m, nil
}

// flattenAsset flattens Asset from a JSON request object into the
// Asset type.
func flattenAsset(c *Client, i interface{}, res *Asset) *Asset {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}
	if len(m) == 0 {
		return nil
	}

	resultRes := &Asset{}
	resultRes.Name = dcl.FlattenString(m["name"])
	resultRes.DisplayName = dcl.FlattenString(m["displayName"])
	resultRes.Uid = dcl.FlattenString(m["uid"])
	resultRes.CreateTime = dcl.FlattenString(m["createTime"])
	resultRes.UpdateTime = dcl.FlattenString(m["updateTime"])
	resultRes.Labels = dcl.FlattenKeyValuePairs(m["labels"])
	resultRes.Description = dcl.FlattenString(m["description"])
	resultRes.State = flattenAssetStateEnum(m["state"])
	resultRes.ResourceSpec = flattenAssetResourceSpec(c, m["resourceSpec"], res)
	resultRes.ResourceStatus = flattenAssetResourceStatus(c, m["resourceStatus"], res)
	resultRes.SecurityStatus = flattenAssetSecurityStatus(c, m["securityStatus"], res)
	resultRes.DiscoverySpec = flattenAssetDiscoverySpec(c, m["discoverySpec"], res)
	resultRes.DiscoveryStatus = flattenAssetDiscoveryStatus(c, m["discoveryStatus"], res)
	resultRes.Project = dcl.FlattenString(m["project"])
	resultRes.Location = dcl.FlattenString(m["location"])
	resultRes.Lake = dcl.FlattenString(m["lake"])
	resultRes.DataplexZone = dcl.FlattenString(m["zone"])

	return resultRes
}

// expandAssetResourceSpec expands an instance of AssetResourceSpec into a JSON
// request object.
func expandAssetResourceSpec(c *Client, f *AssetResourceSpec, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.Name; !dcl.IsEmptyValueIndirect(v) {
		m["name"] = v
	}
	if v := f.Type; !dcl.IsEmptyValueIndirect(v) {
		m["type"] = v
	}
	if v := f.ReadAccessMode; !dcl.IsEmptyValueIndirect(v) {
		m["readAccessMode"] = v
	}

	return m, nil
}

// flattenAssetResourceSpec flattens an instance of AssetResourceSpec from a JSON
// response object.
func flattenAssetResourceSpec(c *Client, i interface{}, res *Asset) *AssetResourceSpec {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetResourceSpec{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetResourceSpec
	}
	r.Name = dcl.FlattenString(m["name"])
	r.Type = flattenAssetResourceSpecTypeEnum(m["type"])
	r.ReadAccessMode = flattenAssetResourceSpecReadAccessModeEnum(m["readAccessMode"])

	return r
}

// expandAssetResourceStatus expands an instance of AssetResourceStatus into a JSON
// request object.
func expandAssetResourceStatus(c *Client, f *AssetResourceStatus, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.State; !dcl.IsEmptyValueIndirect(v) {
		m["state"] = v
	}
	if v := f.Message; !dcl.IsEmptyValueIndirect(v) {
		m["message"] = v
	}
	if v := f.UpdateTime; !dcl.IsEmptyValueIndirect(v) {
		m["updateTime"] = v
	}

	return m, nil
}

// flattenAssetResourceStatus flattens an instance of AssetResourceStatus from a JSON
// response object.
func flattenAssetResourceStatus(c *Client, i interface{}, res *Asset) *AssetResourceStatus {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetResourceStatus{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetResourceStatus
	}
	r.State = flattenAssetResourceStatusStateEnum(m["state"])
	r.Message = dcl.FlattenString(m["message"])
	r.UpdateTime = dcl.FlattenString(m["updateTime"])

	return r
}

// expandAssetSecurityStatus expands an instance of AssetSecurityStatus into a JSON
// request object.
func expandAssetSecurityStatus(c *Client, f *AssetSecurityStatus, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.State; !dcl.IsEmptyValueIndirect(v) {
		m["state"] = v
	}
	if v := f.Message; !dcl.IsEmptyValueIndirect(v) {
		m["message"] = v
	}
	if v := f.UpdateTime; !dcl.IsEmptyValueIndirect(v) {
		m["updateTime"] = v
	}

	return m, nil
}

// flattenAssetSecurityStatus flattens an instance of AssetSecurityStatus from a JSON
// response object.
func flattenAssetSecurityStatus(c *Client, i interface{}, res *Asset) *AssetSecurityStatus {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetSecurityStatus{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetSecurityStatus
	}
	r.State = flattenAssetSecurityStatusStateEnum(m["state"])
	r.Message = dcl.FlattenString(m["message"])
	r.UpdateTime = dcl.FlattenString(m["updateTime"])

	return r
}

// expandAssetDiscoverySpec expands an instance of AssetDiscoverySpec into a JSON
// request object.
func expandAssetDiscoverySpec(c *Client, f *AssetDiscoverySpec, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.Enabled; !dcl.IsEmptyValueIndirect(v) {
		m["enabled"] = v
	}
	if v := f.IncludePatterns; v != nil {
		m["includePatterns"] = v
	}
	if v := f.ExcludePatterns; v != nil {
		m["excludePatterns"] = v
	}
	if v, err := expandAssetDiscoverySpecCsvOptions(c, f.CsvOptions, res); err != nil {
		return nil, fmt.Errorf("error expanding CsvOptions into csvOptions: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["csvOptions"] = v
	}
	if v, err := expandAssetDiscoverySpecJsonOptions(c, f.JsonOptions, res); err != nil {
		return nil, fmt.Errorf("error expanding JsonOptions into jsonOptions: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["jsonOptions"] = v
	}
	if v := f.Schedule; !dcl.IsEmptyValueIndirect(v) {
		m["schedule"] = v
	}

	return m, nil
}

// flattenAssetDiscoverySpec flattens an instance of AssetDiscoverySpec from a JSON
// response object.
func flattenAssetDiscoverySpec(c *Client, i interface{}, res *Asset) *AssetDiscoverySpec {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetDiscoverySpec{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetDiscoverySpec
	}
	r.Enabled = dcl.FlattenBool(m["enabled"])
	r.IncludePatterns = dcl.FlattenStringSlice(m["includePatterns"])
	r.ExcludePatterns = dcl.FlattenStringSlice(m["excludePatterns"])
	r.CsvOptions = flattenAssetDiscoverySpecCsvOptions(c, m["csvOptions"], res)
	r.JsonOptions = flattenAssetDiscoverySpecJsonOptions(c, m["jsonOptions"], res)
	r.Schedule = dcl.FlattenString(m["schedule"])

	return r
}

// expandAssetDiscoverySpecCsvOptions expands an instance of AssetDiscoverySpecCsvOptions into a JSON
// request object.
func expandAssetDiscoverySpecCsvOptions(c *Client, f *AssetDiscoverySpecCsvOptions, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.HeaderRows; !dcl.IsEmptyValueIndirect(v) {
		m["headerRows"] = v
	}
	if v := f.Delimiter; !dcl.IsEmptyValueIndirect(v) {
		m["delimiter"] = v
	}
	if v := f.Encoding; !dcl.IsEmptyValueIndirect(v) {
		m["encoding"] = v
	}
	if v := f.DisableTypeInference; !dcl.IsEmptyValueIndirect(v) {
		m["disableTypeInference"] = v
	}

	return m, nil
}

// flattenAssetDiscoverySpecCsvOptions flattens an instance of AssetDiscoverySpecCsvOptions from a JSON
// response object.
func flattenAssetDiscoverySpecCsvOptions(c *Client, i interface{}, res *Asset) *AssetDiscoverySpecCsvOptions {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetDiscoverySpecCsvOptions{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetDiscoverySpecCsvOptions
	}
	r.HeaderRows = dcl.FlattenInteger(m["headerRows"])
	r.Delimiter = dcl.FlattenString(m["delimiter"])
	r.Encoding = dcl.FlattenString(m["encoding"])
	r.DisableTypeInference = dcl.FlattenBool(m["disableTypeInference"])

	return r
}

// expandAssetDiscoverySpecJsonOptions expands an instance of AssetDiscoverySpecJsonOptions into a JSON
// request object.
func expandAssetDiscoverySpecJsonOptions(c *Client, f *AssetDiscoverySpecJsonOptions, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.Encoding; !dcl.IsEmptyValueIndirect(v) {
		m["encoding"] = v
	}
	if v := f.DisableTypeInference; !dcl.IsEmptyValueIndirect(v) {
		m["disableTypeInference"] = v
	}

	return m, nil
}

// flattenAssetDiscoverySpecJsonOptions flattens an instance of AssetDiscoverySpecJsonOptions from a JSON
// response object.
func flattenAssetDiscoverySpecJsonOptions(c *Client, i interface{}, res *Asset) *AssetDiscoverySpecJsonOptions {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetDiscoverySpecJsonOptions{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetDiscoverySpecJsonOptions
	}
	r.Encoding = dcl.FlattenString(m["encoding"])
	r.DisableTypeInference = dcl.FlattenBool(m["disableTypeInference"])

	return r
}

// expandAssetDiscoveryStatus expands an instance of AssetDiscoveryStatus into a JSON
// request object.
func expandAssetDiscoveryStatus(c *Client, f *AssetDiscoveryStatus, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.State; !dcl.IsEmptyValueIndirect(v) {
		m["state"] = v
	}
	if v := f.Message; !dcl.IsEmptyValueIndirect(v) {
		m["message"] = v
	}
	if v := f.UpdateTime; !dcl.IsEmptyValueIndirect(v) {
		m["updateTime"] = v
	}
	if v := f.LastRunTime; !dcl.IsEmptyValueIndirect(v) {
		m["lastRunTime"] = v
	}
	if v, err := expandAssetDiscoveryStatusStats(c, f.Stats, res); err != nil {
		return nil, fmt.Errorf("error expanding Stats into stats: %w", err)
	} else if !dcl.IsEmptyValueIndirect(v) {
		m["stats"] = v
	}
	if v := f.LastRunDuration; !dcl.IsEmptyValueIndirect(v) {
		m["lastRunDuration"] = v
	}

	return m, nil
}

// flattenAssetDiscoveryStatus flattens an instance of AssetDiscoveryStatus from a JSON
// response object.
func flattenAssetDiscoveryStatus(c *Client, i interface{}, res *Asset) *AssetDiscoveryStatus {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetDiscoveryStatus{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetDiscoveryStatus
	}
	r.State = flattenAssetDiscoveryStatusStateEnum(m["state"])
	r.Message = dcl.FlattenString(m["message"])
	r.UpdateTime = dcl.FlattenString(m["updateTime"])
	r.LastRunTime = dcl.FlattenString(m["lastRunTime"])
	r.Stats = flattenAssetDiscoveryStatusStats(c, m["stats"], res)
	r.LastRunDuration = dcl.FlattenString(m["lastRunDuration"])

	return r
}

// expandAssetDiscoveryStatusStats expands an instance of AssetDiscoveryStatusStats into a JSON
// request object.
func expandAssetDiscoveryStatusStats(c *Client, f *AssetDiscoveryStatusStats, res *Asset) (map[string]interface{}, error) {
	if dcl.IsEmptyValueIndirect(f) {
		return nil, nil
	}

	m := make(map[string]interface{})
	if v := f.DataItems; !dcl.IsEmptyValueIndirect(v) {
		m["dataItems"] = v
	}
	if v := f.DataSize; !dcl.IsEmptyValueIndirect(v) {
		m["dataSize"] = v
	}
	if v := f.Tables; !dcl.IsEmptyValueIndirect(v) {
		m["tables"] = v
	}
	if v := f.Filesets; !dcl.IsEmptyValueIndirect(v) {
		m["filesets"] = v
	}

	return m, nil
}

// flattenAssetDiscoveryStatusStats flattens an instance of AssetDiscoveryStatusStats from a JSON
// response object.
func flattenAssetDiscoveryStatusStats(c *Client, i interface{}, res *Asset) *AssetDiscoveryStatusStats {
	m, ok := i.(map[string]interface{})
	if !ok {
		return nil
	}

	r := &AssetDiscoveryStatusStats{}

	if dcl.IsEmptyValueIndirect(i) {
		return EmptyAssetDiscoveryStatusStats
	}
	r.DataItems = dcl.FlattenInteger(m["dataItems"])
	r.DataSize = dcl.FlattenInteger(m["dataSize"])
	r.Tables = dcl.FlattenInteger(m["tables"])
	r.Filesets = dcl.FlattenInteger(m["filesets"])

	return r
}

// flattenAssetStateEnum asserts that an interface is a string, and returns a
// pointer to a *AssetStateEnum with the same value as that string.
func flattenAssetStateEnum(i interface{}) *AssetStateEnum {
	s, ok := i.(string)
	if !ok {
		return nil
	}

	return AssetStateEnumRef(s)
}

// flattenAssetResourceSpecTypeEnum asserts that an interface is a string, and returns a
// pointer to a *AssetResourceSpecTypeEnum with the same value as that string.
func flattenAssetResourceSpecTypeEnum(i interface{}) *AssetResourceSpecTypeEnum {
	s, ok := i.(string)
	if !ok {
		return nil
	}

	return AssetResourceSpecTypeEnumRef(s)
}

// flattenAssetResourceSpecReadAccessModeEnum asserts that an interface is a string, and returns a
// pointer to a *AssetResourceSpecReadAccessModeEnum with the same value as that string.
func flattenAssetResourceSpecReadAccessModeEnum(i interface{}) *AssetResourceSpecReadAccessModeEnum {
	s, ok := i.(string)
	if !ok {
		return nil
	}

	return AssetResourceSpecReadAccessModeEnumRef(s)
}

// flattenAssetResourceStatusStateEnum asserts that an interface is a string, and returns a
// pointer to a *AssetResourceStatusStateEnum with the same value as that string.
func flattenAssetResourceStatusStateEnum(i interface{}) *AssetResourceStatusStateEnum {
	s, ok := i.(string)
	if !ok {
		return nil
	}

	return AssetResourceStatusStateEnumRef(s)
}

// flattenAssetSecurityStatusStateEnum asserts that an interface is a string, and returns a
// pointer to a *AssetSecurityStatusStateEnum with the same value as that string.
func flattenAssetSecurityStatusStateEnum(i interface{}) *AssetSecurityStatusStateEnum {
	s, ok := i.(string)
	if !ok {
		return nil
	}

	return AssetSecurityStatusStateEnumRef(s)
}

// flattenAssetDiscoveryStatusStateEnum asserts that an interface is a string, and returns a
// pointer to a *AssetDiscoveryStatusStateEnum with the same value as that string.
func flattenAssetDiscoveryStatusStateEnum(i interface{}) *AssetDiscoveryStatusStateEnum {
	s, ok := i.(string)
	if !ok {
		return nil
	}

	return AssetDiscoveryStatusStateEnumRef(s)
}

type assetDiff struct {
	// The diff should include one or the other of RequiresRecreate or UpdateOp.
	RequiresRecreate bool
	UpdateOp         assetApiOperation
	FieldName        string // used for error logging
}

func convertFieldDiffsToAssetDiffs(config *dcl.Config, fds []*dcl.FieldDiff, opts []dcl.ApplyOption) ([]assetDiff, error) {
	opNamesToFieldDiffs := make(map[string][]*dcl.FieldDiff)
	// Map each operation name to the field diffs associated with it.
	for _, fd := range fds {
		for _, ro := range fd.ResultingOperation {
			if fieldDiffs, ok := opNamesToFieldDiffs[ro]; ok {
				fieldDiffs = append(fieldDiffs, fd)
				opNamesToFieldDiffs[ro] = fieldDiffs
			} else {
				config.Logger.Infof("%s required due to diff: %v", ro, fd)
				opNamesToFieldDiffs[ro] = []*dcl.FieldDiff{fd}
			}
		}
	}
	var diffs []assetDiff
	// For each operation name, create a assetDiff which contains the operation.
	for opName, fieldDiffs := range opNamesToFieldDiffs {
		// Use the first field diff's field name for logging required recreate error.
		diff := assetDiff{FieldName: fieldDiffs[0].FieldName}
		if opName == "Recreate" {
			diff.RequiresRecreate = true
		} else {
			apiOp, err := convertOpNameToAssetApiOperation(opName, fieldDiffs, opts...)
			if err != nil {
				return diffs, err
			}
			diff.UpdateOp = apiOp
		}
		diffs = append(diffs, diff)
	}
	return diffs, nil
}

func convertOpNameToAssetApiOperation(opName string, fieldDiffs []*dcl.FieldDiff, opts ...dcl.ApplyOption) (assetApiOperation, error) {
	switch opName {

	case "updateAssetUpdateAssetOperation":
		return &updateAssetUpdateAssetOperation{FieldDiffs: fieldDiffs}, nil

	default:
		return nil, fmt.Errorf("no such operation with name: %v", opName)
	}
}

func extractAssetFields(r *Asset) error {
	vResourceSpec := r.ResourceSpec
	if vResourceSpec == nil {
		// note: explicitly not the empty object.
		vResourceSpec = &AssetResourceSpec{}
	}
	if err := extractAssetResourceSpecFields(r, vResourceSpec); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vResourceSpec) {
		r.ResourceSpec = vResourceSpec
	}
	vResourceStatus := r.ResourceStatus
	if vResourceStatus == nil {
		// note: explicitly not the empty object.
		vResourceStatus = &AssetResourceStatus{}
	}
	if err := extractAssetResourceStatusFields(r, vResourceStatus); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vResourceStatus) {
		r.ResourceStatus = vResourceStatus
	}
	vSecurityStatus := r.SecurityStatus
	if vSecurityStatus == nil {
		// note: explicitly not the empty object.
		vSecurityStatus = &AssetSecurityStatus{}
	}
	if err := extractAssetSecurityStatusFields(r, vSecurityStatus); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vSecurityStatus) {
		r.SecurityStatus = vSecurityStatus
	}
	vDiscoverySpec := r.DiscoverySpec
	if vDiscoverySpec == nil {
		// note: explicitly not the empty object.
		vDiscoverySpec = &AssetDiscoverySpec{}
	}
	if err := extractAssetDiscoverySpecFields(r, vDiscoverySpec); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vDiscoverySpec) {
		r.DiscoverySpec = vDiscoverySpec
	}
	vDiscoveryStatus := r.DiscoveryStatus
	if vDiscoveryStatus == nil {
		// note: explicitly not the empty object.
		vDiscoveryStatus = &AssetDiscoveryStatus{}
	}
	if err := extractAssetDiscoveryStatusFields(r, vDiscoveryStatus); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vDiscoveryStatus) {
		r.DiscoveryStatus = vDiscoveryStatus
	}
	return nil
}
func extractAssetResourceSpecFields(r *Asset, o *AssetResourceSpec) error {
	return nil
}
func extractAssetResourceStatusFields(r *Asset, o *AssetResourceStatus) error {
	return nil
}
func extractAssetSecurityStatusFields(r *Asset, o *AssetSecurityStatus) error {
	return nil
}
func extractAssetDiscoverySpecFields(r *Asset, o *AssetDiscoverySpec) error {
	vCsvOptions := o.CsvOptions
	if vCsvOptions == nil {
		// note: explicitly not the empty object.
		vCsvOptions = &AssetDiscoverySpecCsvOptions{}
	}
	if err := extractAssetDiscoverySpecCsvOptionsFields(r, vCsvOptions); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vCsvOptions) {
		o.CsvOptions = vCsvOptions
	}
	vJsonOptions := o.JsonOptions
	if vJsonOptions == nil {
		// note: explicitly not the empty object.
		vJsonOptions = &AssetDiscoverySpecJsonOptions{}
	}
	if err := extractAssetDiscoverySpecJsonOptionsFields(r, vJsonOptions); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vJsonOptions) {
		o.JsonOptions = vJsonOptions
	}
	return nil
}
func extractAssetDiscoverySpecCsvOptionsFields(r *Asset, o *AssetDiscoverySpecCsvOptions) error {
	return nil
}
func extractAssetDiscoverySpecJsonOptionsFields(r *Asset, o *AssetDiscoverySpecJsonOptions) error {
	return nil
}
func extractAssetDiscoveryStatusFields(r *Asset, o *AssetDiscoveryStatus) error {
	vStats := o.Stats
	if vStats == nil {
		// note: explicitly not the empty object.
		vStats = &AssetDiscoveryStatusStats{}
	}
	if err := extractAssetDiscoveryStatusStatsFields(r, vStats); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vStats) {
		o.Stats = vStats
	}
	return nil
}
func extractAssetDiscoveryStatusStatsFields(r *Asset, o *AssetDiscoveryStatusStats) error {
	return nil
}

func postReadExtractAssetFields(r *Asset) error {
	vResourceSpec := r.ResourceSpec
	if vResourceSpec == nil {
		// note: explicitly not the empty object.
		vResourceSpec = &AssetResourceSpec{}
	}
	if err := postReadExtractAssetResourceSpecFields(r, vResourceSpec); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vResourceSpec) {
		r.ResourceSpec = vResourceSpec
	}
	vResourceStatus := r.ResourceStatus
	if vResourceStatus == nil {
		// note: explicitly not the empty object.
		vResourceStatus = &AssetResourceStatus{}
	}
	if err := postReadExtractAssetResourceStatusFields(r, vResourceStatus); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vResourceStatus) {
		r.ResourceStatus = vResourceStatus
	}
	vSecurityStatus := r.SecurityStatus
	if vSecurityStatus == nil {
		// note: explicitly not the empty object.
		vSecurityStatus = &AssetSecurityStatus{}
	}
	if err := postReadExtractAssetSecurityStatusFields(r, vSecurityStatus); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vSecurityStatus) {
		r.SecurityStatus = vSecurityStatus
	}
	vDiscoverySpec := r.DiscoverySpec
	if vDiscoverySpec == nil {
		// note: explicitly not the empty object.
		vDiscoverySpec = &AssetDiscoverySpec{}
	}
	if err := postReadExtractAssetDiscoverySpecFields(r, vDiscoverySpec); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vDiscoverySpec) {
		r.DiscoverySpec = vDiscoverySpec
	}
	vDiscoveryStatus := r.DiscoveryStatus
	if vDiscoveryStatus == nil {
		// note: explicitly not the empty object.
		vDiscoveryStatus = &AssetDiscoveryStatus{}
	}
	if err := postReadExtractAssetDiscoveryStatusFields(r, vDiscoveryStatus); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vDiscoveryStatus) {
		r.DiscoveryStatus = vDiscoveryStatus
	}
	return nil
}
func postReadExtractAssetResourceSpecFields(r *Asset, o *AssetResourceSpec) error {
	return nil
}
func postReadExtractAssetResourceStatusFields(r *Asset, o *AssetResourceStatus) error {
	return nil
}
func postReadExtractAssetSecurityStatusFields(r *Asset, o *AssetSecurityStatus) error {
	return nil
}
func postReadExtractAssetDiscoverySpecFields(r *Asset, o *AssetDiscoverySpec) error {
	vCsvOptions := o.CsvOptions
	if vCsvOptions == nil {
		// note: explicitly not the empty object.
		vCsvOptions = &AssetDiscoverySpecCsvOptions{}
	}
	if err := extractAssetDiscoverySpecCsvOptionsFields(r, vCsvOptions); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vCsvOptions) {
		o.CsvOptions = vCsvOptions
	}
	vJsonOptions := o.JsonOptions
	if vJsonOptions == nil {
		// note: explicitly not the empty object.
		vJsonOptions = &AssetDiscoverySpecJsonOptions{}
	}
	if err := extractAssetDiscoverySpecJsonOptionsFields(r, vJsonOptions); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vJsonOptions) {
		o.JsonOptions = vJsonOptions
	}
	return nil
}
func postReadExtractAssetDiscoveryStatusFields(r *Asset, o *AssetDiscoveryStatus) error {
	vStats := o.Stats
	if vStats == nil {
		// note: explicitly not the empty object.
		vStats = &AssetDiscoveryStatusStats{}
	}
	if err := extractAssetDiscoveryStatusStatsFields(r, vStats); err != nil {
		return err
	}
	if !dcl.IsEmptyValueIndirect(vStats) {
		o.Stats = vStats
	}
	return nil
}
