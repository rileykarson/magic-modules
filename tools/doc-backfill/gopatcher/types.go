package gopatcher

type SDKKind string

const (
	SDKv2           SDKKind = "sdkv2"
	PluginFramework SDKKind = "framework"
	IAM             SDKKind = "iam"
)

type byteSpan struct {
	start int
	end   int
}

type compositePatchMeta struct {
	filePath     string
	compStart    int
	lbraceOffset int
	rbraceOffset int
	descSpans    []byteSpan
	insertOffset int
	insertPrefix string
	insertSuffix string
}

type FieldTarget struct {
	Path               string // canonical dot-separated path, e.g. "boot_disk.initialize_params.size"
	FilePath           string // path to .go or .go.tmpl file
	TargetID           string // unique deterministic key "<FilePath>:<CompositeLitStartOffset>" to detect shared helpers across resources
	HasDescription     bool   // true if Description (or MarkdownDescription) key is present in composite literal
	CurrentDescription string // current string value if literal ("" if missing or non-literal)
	IsLiteralOrMissing bool   // true if missing OR a single string literal (raw/interpreted); false if complex expr (func call, var, concat)
	SharedHelper       string // helper func/var name if resolved outside the resource's root func, else ""

	patchMeta compositePatchMeta
}

type ResourceTarget struct {
	ResourceName      string                    // e.g. "google_compute_instance", or "google_storage_bucket_iam" for IAM parent schema
	Service           string                    // directory name under services/, e.g. "compute"
	Kind              SDKKind                   // SDKv2, PluginFramework, or IAM
	PrimaryFile       string                    // file declaring the resource registration
	HasResDescription bool                      // true if resource-level Description/MarkdownDescription is set (Plugin Framework & SDKv2 Description)
	CurrentResDesc    string                    // current resource-level description if literal
	CanPatchResDesc   bool                      // true if resource-level Description can be safely patched (e.g. Plugin Framework resp.Schema)
	Fields            map[string][]*FieldTarget // slice supports .go.tmpl if/else branches defining the same field key in both branches

	resPatchMeta compositePatchMeta
}

type PatchRequest struct {
	Target         *FieldTarget
	NewDescription string
}

type ResourceDescPatch struct {
	Resource       *ResourceTarget
	NewDescription string
}

type FileDiff struct {
	FilePath      string
	OriginalBytes []byte
	UpdatedBytes  []byte
	AppliedCount  int
}
