package resourcemanager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/netip"
	"sort"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

type googRanges struct {
	SyncToken    string     `json:"syncToken"`
	CreationTime string     `json:"creationTime"`
	Prefixes     []prefixes `json:"prefixes"`
}

type prefixes struct {
	Ipv4Prefix string `json:"ipv4Prefix"`
	Ipv6Prefix string `json:"ipv6Prefix"`
}

const (
	CLOUD_NETBLOCK_URL  = "https://www.gstatic.com/ipranges/cloud.json"
	GOOGLE_NETBLOCK_URL = "https://www.gstatic.com/ipranges/goog.json"
)

func DataSourceGoogleNetblockIpRanges() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceGoogleNetblockIpRangesRead,

		Schema: map[string]*schema.Schema{
			"range_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "cloud-netblocks",
			},
			"cidr_blocks": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
			},
			"cidr_blocks_ipv4": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
			},
			"cidr_blocks_ipv6": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
			},
		},
	}
}

func dataSourceGoogleNetblockIpRangesRead(d *schema.ResourceData, meta interface{}) error {

	rt := d.Get("range_type").(string)
	CidrBlocks := make(map[string][]string)

	switch rt {
	// Dynamic ranges
	case "cloud-netblocks":
		// https://cloud.google.com/compute/docs/faq#find_ip_range
		CidrBlocks, err := getCidrBlocksFromUrl(CLOUD_NETBLOCK_URL)

		if err != nil {
			return err
		}
		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	case "google-netblocks":
		// https://cloud.google.com/vpc/docs/configure-private-google-access?hl=en#ip-addr-defaults
		CidrBlocks, err := getCidrBlocksFromUrl(GOOGLE_NETBLOCK_URL)

		if err != nil {
			return err
		}
		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	case "default-domains-netblocks":
		// https: //cloud.google.com/vpc/docs/configure-private-google-access#ip-addr-defaults
		googleBlocks, err := getCidrBlocksFromUrl(GOOGLE_NETBLOCK_URL)
		if err != nil {
			return err
		}
		cloudBlocks, err := getCidrBlocksFromUrl(CLOUD_NETBLOCK_URL)
		if err != nil {
			return err
		}
		CidrBlocks, err := getCidrsDifference(googleBlocks, cloudBlocks)
		if err != nil {
			return err
		}

		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	// Static ranges
	case "restricted-googleapis":
		// https://cloud.google.com/vpc/docs/private-access-options#domain-vips
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "199.36.153.4/30")
		CidrBlocks["cidr_blocks_ipv6"] = append(CidrBlocks["cidr_blocks_ipv6"], "2600:2d00:0002:1000::/64")
		CidrBlocks["cidr_blocks"] = append(CidrBlocks["cidr_blocks_ipv4"], CidrBlocks["cidr_blocks_ipv6"]...)

		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	case "restricted-googleapis-with-directconnectivity":
		// https://cloud.google.com/vpc/docs/configure-private-google-access#config-options
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "199.36.153.4/30", "34.126.0.0/18")
		CidrBlocks["cidr_blocks_ipv6"] = append(CidrBlocks["cidr_blocks_ipv6"], "2600:2d00:0002:1000::/64", "2001:4860:8040::/42")
		CidrBlocks["cidr_blocks"] = append(CidrBlocks["cidr_blocks_ipv4"], CidrBlocks["cidr_blocks_ipv6"]...)

		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	case "private-googleapis":
		// https://cloud.google.com/vpc/docs/private-access-options#domain-vips
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "199.36.153.8/30")
		CidrBlocks["cidr_blocks_ipv6"] = append(CidrBlocks["cidr_blocks_ipv6"], "2600:2d00:0002:2000::/64")
		CidrBlocks["cidr_blocks"] = append(CidrBlocks["cidr_blocks_ipv4"], CidrBlocks["cidr_blocks_ipv6"]...)

		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	case "private-googleapis-with-directconnectivity":
		// https://cloud.google.com/vpc/docs/private-access-options#domain-vips
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "199.36.153.8/30", "34.126.0.0/18")
		CidrBlocks["cidr_blocks_ipv6"] = append(CidrBlocks["cidr_blocks_ipv6"], "2600:2d00:0002:2000::/64", "2001:4860:8040::/42")
		CidrBlocks["cidr_blocks"] = append(CidrBlocks["cidr_blocks_ipv4"], CidrBlocks["cidr_blocks_ipv6"]...)

		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv6", CidrBlocks["cidr_blocks_ipv6"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv6: %s", err)
		}
	case "dns-forwarders":
		// https://cloud.google.com/dns/zones/#creating-forwarding-zones
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "35.199.192.0/19")
		CidrBlocks["cidr_blocks"] = CidrBlocks["cidr_blocks_ipv4"]
		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
	case "iap-forwarders":
		// https://cloud.google.com/iap/docs/using-tcp-forwarding
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "35.235.240.0/20")
		CidrBlocks["cidr_blocks"] = CidrBlocks["cidr_blocks_ipv4"]
		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
	case "health-checkers":
		// https://cloud.google.com/load-balancing/docs/health-checks#fw-ruleh
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "35.191.0.0/16")
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "130.211.0.0/22")
		CidrBlocks["cidr_blocks"] = CidrBlocks["cidr_blocks_ipv4"]
		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
	case "legacy-health-checkers":
		// https://cloud.google.com/load-balancing/docs/health-check#fw-netlbs
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "35.191.0.0/16")
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "209.85.152.0/22")
		CidrBlocks["cidr_blocks_ipv4"] = append(CidrBlocks["cidr_blocks_ipv4"], "209.85.204.0/22")
		CidrBlocks["cidr_blocks"] = CidrBlocks["cidr_blocks_ipv4"]
		if err := d.Set("cidr_blocks", CidrBlocks["cidr_blocks"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks: %s", err)
		}
		if err := d.Set("cidr_blocks_ipv4", CidrBlocks["cidr_blocks_ipv4"]); err != nil {
			return fmt.Errorf("Error setting cidr_blocks_ipv4: %s", err)
		}
	default:
		return fmt.Errorf("Unknown range_type: %s", rt)
	}

	d.SetId("netblock-ip-ranges-" + rt)

	return nil
}

func getCidrBlocksFromUrl(url string) (map[string][]string, error) {
	cidrBlocks := make(map[string][]string)

	response, err := http.Get(url)

	if err != nil {
		return nil, fmt.Errorf("Error: %s", err)
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)

	if err != nil {
		return nil, fmt.Errorf("Error to retrieve the CIDR list: %s", err)
	}

	ranges := googRanges{}
	jsonErr := json.Unmarshal(body, &ranges)
	if jsonErr != nil {
		return nil, fmt.Errorf("Error reading JSON list: %s", jsonErr)
	}

	for _, element := range ranges.Prefixes {

		if len(element.Ipv4Prefix) > 0 {
			cidrBlocks["cidr_blocks_ipv4"] = append(cidrBlocks["cidr_blocks_ipv4"], element.Ipv4Prefix)
			cidrBlocks["cidr_blocks"] = append(cidrBlocks["cidr_blocks"], element.Ipv4Prefix)
		} else if len(element.Ipv6Prefix) > 0 {
			cidrBlocks["cidr_blocks_ipv6"] = append(cidrBlocks["cidr_blocks_ipv6"], element.Ipv6Prefix)
			cidrBlocks["cidr_blocks"] = append(cidrBlocks["cidr_blocks"], element.Ipv6Prefix)
		}

	}

	return cidrBlocks, nil
}

func getCidrsDifference(reference, excluded map[string][]string) (map[string][]string, error) {
	result := make(map[string][]string)

	for blockName := range reference {
		ipRangeStrings, err := cidrBlocksMinus(reference[blockName], excluded[blockName])
		if err != nil {
			return result, err
		}
		result[blockName] = ipRangeStrings
	}

	return result, nil
}

// CidrBlocksMinus returns the sorted CIDR blocks remaining after subtracting subtract from target.
func CidrBlocksMinus(target, subtract []string) ([]string, error) {
	return cidrBlocksMinus(target, subtract)
}

func cidrBlocksMinus(target, subtract []string) ([]string, error) {
	var current []netip.Prefix
	for _, cidr := range target {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, err
		}
		current = append(current, p.Masked())
	}
	current = coalescePrefixes(current)

	for _, cidr := range subtract {
		sub, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, err
		}
		sub = sub.Masked()
		var next []netip.Prefix
		for _, p := range current {
			next = append(next, subtractPrefix(p, sub)...)
		}
		current = next
	}

	var ipRangeStrings []string
	for _, p := range current {
		ipRangeStrings = append(ipRangeStrings, p.String())
	}
	sort.Strings(ipRangeStrings)
	return ipRangeStrings, nil
}

func subtractPrefix(p, sub netip.Prefix) []netip.Prefix {
	if !p.Overlaps(sub) {
		return []netip.Prefix{p}
	}
	if sub.Bits() <= p.Bits() {
		return nil
	}
	left, right := splitPrefix(p)
	return append(subtractPrefix(left, sub), subtractPrefix(right, sub)...)
}

func splitPrefix(p netip.Prefix) (netip.Prefix, netip.Prefix) {
	b := p.Bits()
	left := netip.PrefixFrom(p.Addr(), b+1)
	if p.Addr().Is4() {
		a4 := p.Addr().As4()
		a4[b/8] |= 1 << (7 - (b % 8))
		return left, netip.PrefixFrom(netip.AddrFrom4(a4), b+1)
	}
	a16 := p.Addr().As16()
	a16[b/8] |= 1 << (7 - (b % 8))
	return left, netip.PrefixFrom(netip.AddrFrom16(a16), b+1)
}

func coalescePrefixes(prefixes []netip.Prefix) []netip.Prefix {
	if len(prefixes) == 0 {
		return nil
	}
	sort.Slice(prefixes, func(i, j int) bool {
		if prefixes[i].Addr().BitLen() != prefixes[j].Addr().BitLen() {
			return prefixes[i].Addr().BitLen() < prefixes[j].Addr().BitLen()
		}
		if cmp := prefixes[i].Addr().Compare(prefixes[j].Addr()); cmp != 0 {
			return cmp < 0
		}
		return prefixes[i].Bits() < prefixes[j].Bits()
	})

	var dedup []netip.Prefix
	for _, p := range prefixes {
		if len(dedup) > 0 && dedup[len(dedup)-1].Overlaps(p) {
			continue
		}
		dedup = append(dedup, p)
	}

	var merged []netip.Prefix
	for _, p := range dedup {
		for len(merged) > 0 {
			top := merged[len(merged)-1]
			if top.Addr().BitLen() != p.Addr().BitLen() || top.Bits() != p.Bits() || p.Bits() == 0 {
				break
			}
			parent := netip.PrefixFrom(top.Addr(), p.Bits()-1).Masked()
			if !parent.Overlaps(p) {
				break
			}
			merged = merged[:len(merged)-1]
			p = parent
		}
		merged = append(merged, p)
	}
	return merged
}

func init() {
	registry.Schema{
		Name:        "google_netblock_ip_ranges",
		ProductName: "resourcemanager",
		Type:        registry.SchemaTypeDataSource,
		Schema:      DataSourceGoogleNetblockIpRanges(),
	}.Register()
}
