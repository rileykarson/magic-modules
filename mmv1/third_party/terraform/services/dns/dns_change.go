package dns

import (
	"os"
	"time"

	"google.golang.org/api/dns/v1"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type DnsChangeWaiter struct {
	Service     *dns.Service
	Change      *dns.Change
	Project     string
	ManagedZone string
}

func (w *DnsChangeWaiter) RefreshFunc() retry.StateRefreshFunc {
	return func() (interface{}, string, error) {
		var chg *dns.Change
		var err error

		chg, err = w.Service.Changes.Get(
			w.Project, w.ManagedZone, w.Change.Id).Do()

		if err != nil {
			return nil, "", err
		}

		return chg, chg.Status, nil
	}
}

func (w *DnsChangeWaiter) Conf() *retry.StateChangeConf {
	minTimeout := 2 * time.Second
	var pollInterval time.Duration
	if os.Getenv("VCR_PATH") != "" && os.Getenv("VCR_MODE") == "REPLAYING" {
		minTimeout = time.Millisecond
		pollInterval = time.Millisecond
	}
	return &retry.StateChangeConf{
		Pending:      []string{"pending"},
		Target:       []string{"done"},
		Refresh:      w.RefreshFunc(),
		Timeout:      10 * time.Minute,
		MinTimeout:   minTimeout,
		PollInterval: pollInterval,
	}
}
