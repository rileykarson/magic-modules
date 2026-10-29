package resourcemanager_test

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/services/resourcemanager"
)

func TestCidrBlocksMinus(t *testing.T) {
	cases := map[string]struct {
		target      []string
		subtract    []string
		want        []string
		expectError bool
	}{
		"empty target": {
			target:   nil,
			subtract: []string{"10.0.0.0/24"},
			want:     nil,
		},
		"IPv4 disjoint ranges": {
			target:   []string{"10.0.0.0/24"},
			subtract: []string{"10.0.1.0/24", "192.168.0.0/16"},
			want:     []string{"10.0.0.0/24"},
		},
		"IPv4 full containment exact": {
			target:   []string{"10.0.0.0/24"},
			subtract: []string{"10.0.0.0/24"},
			want:     nil,
		},
		"IPv4 full containment superset": {
			target:   []string{"10.0.0.0/24", "10.0.1.0/24"},
			subtract: []string{"10.0.0.0/16"},
			want:     nil,
		},
		"IPv4 single level split left half removed": {
			target:   []string{"10.0.0.0/24"},
			subtract: []string{"10.0.0.0/25"},
			want:     []string{"10.0.0.128/25"},
		},
		"IPv4 single level split right half removed": {
			target:   []string{"10.0.0.0/24"},
			subtract: []string{"10.0.0.128/25"},
			want:     []string{"10.0.0.0/25"},
		},
		"IPv4 multi-level partial split": {
			target:   []string{"10.0.0.0/24"},
			subtract: []string{"10.0.0.64/26"},
			want:     []string{"10.0.0.0/26", "10.0.0.128/25"},
		},
		"IPv4 multiple subtract blocks": {
			target:   []string{"10.0.0.0/24"},
			subtract: []string{"10.0.0.0/26", "10.0.0.192/26"},
			want:     []string{"10.0.0.128/26", "10.0.0.64/26"},
		},
		"IPv4 coalesces sibling target prefixes before subtraction": {
			target:   []string{"10.0.0.0/25", "10.0.0.128/25"},
			subtract: []string{"10.0.0.0/26"},
			want:     []string{"10.0.0.128/25", "10.0.0.64/26"},
		},
		"IPv4 overlapping and duplicate target prefixes": {
			target:   []string{"10.0.0.0/24", "10.0.0.0/25", "10.0.0.0/24"},
			subtract: []string{"10.0.0.128/25"},
			want:     []string{"10.0.0.0/25"},
		},
		"IPv4 /0 prefix splitting": {
			target:   []string{"0.0.0.0/0"},
			subtract: []string{"128.0.0.0/1"},
			want:     []string{"0.0.0.0/1"},
		},
		"IPv6 /0 prefix splitting": {
			target:   []string{"::/0"},
			subtract: []string{"8000::/1"},
			want:     []string{"::/1"},
		},
		"IPv4 /32 host prefix as target and subtract": {
			target:   []string{"10.0.0.1/32", "10.0.0.2/32"},
			subtract: []string{"10.0.0.1/32"},
			want:     []string{"10.0.0.2/32"},
		},
		"IPv4 /32 host prefix subtracted from /30": {
			target:   []string{"10.0.0.0/30"},
			subtract: []string{"10.0.0.1/32"},
			want:     []string{"10.0.0.0/32", "10.0.0.2/31"},
		},
		"IPv6 /128 host prefix as target and subtract": {
			target:   []string{"2001:db8::1/128", "2001:db8::2/128"},
			subtract: []string{"2001:db8::1/128"},
			want:     []string{"2001:db8::2/128"},
		},
		"IPv6 /128 host prefix subtracted from /126": {
			target:   []string{"2001:db8::/126"},
			subtract: []string{"2001:db8::1/128"},
			want:     []string{"2001:db8::/128", "2001:db8::2/127"},
		},
		"IPv6 disjoint ranges": {
			target:   []string{"2001:db8::/32"},
			subtract: []string{"2001:db9::/32"},
			want:     []string{"2001:db8::/32"},
		},
		"IPv6 full containment": {
			target:   []string{"2001:db8:1000::/36"},
			subtract: []string{"2001:db8::/32"},
			want:     nil,
		},
		"IPv6 partial split": {
			target:   []string{"2001:db8::/32"},
			subtract: []string{"2001:db8::/34"},
			want:     []string{"2001:db8:4000::/34", "2001:db8:8000::/33"},
		},
		"mixed IPv4 and IPv6 blocks": {
			target:   []string{"10.0.0.0/24", "2001:db8::/32"},
			subtract: []string{"10.0.0.128/25", "2001:db8::/33"},
			want:     []string{"10.0.0.0/25", "2001:db8:8000::/33"},
		},
		"invalid target CIDR returns error": {
			target:      []string{"not-a-cidr"},
			subtract:    []string{"10.0.0.0/24"},
			expectError: true,
		},
		"invalid subtract CIDR returns error": {
			target:      []string{"10.0.0.0/24"},
			subtract:    []string{"not-a-cidr"},
			expectError: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := resourcemanager.CidrBlocksMinus(tc.target, tc.subtract)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil (result: %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CidrBlocksMinus(%v, %v) = %v, want %v", tc.target, tc.subtract, got, tc.want)
			}
		})
	}
}

func TestAccDataSourceGoogleNetblockIpRanges_basic(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNetblockIpRangesConfig,
				Check: resource.ComposeTestCheckFunc(
					// Cloud netblocks
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.cloud",
						"cidr_blocks.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.cloud",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.cloud",
						"cidr_blocks_ipv4.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.cloud",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.cloud",
						"cidr_blocks_ipv6.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.cloud",
						"cidr_blocks_ipv6.0", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_google,
				Check: resource.ComposeTestCheckFunc(
					// Google netblocks
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.google",
						"cidr_blocks.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.google",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.google",
						"cidr_blocks_ipv4.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.google",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.google",
						"cidr_blocks_ipv6.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.google",
						"cidr_blocks_ipv6.0", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_defaultdomains,
				Check: resource.ComposeTestCheckFunc(
					// Default domains netblocks
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.defaultdomains",
						"cidr_blocks.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.defaultdomains",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.defaultdomains",
						"cidr_blocks_ipv4.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.defaultdomains",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.defaultdomains",
						"cidr_blocks_ipv6.#", regexp.MustCompile(("^[1-9]+[0-9]*$"))),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.defaultdomains",
						"cidr_blocks_ipv6.0", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_restricted,
				Check: resource.ComposeTestCheckFunc(
					// Private Google Access Restricted VIP
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.restricted", "cidr_blocks.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.restricted",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.restricted", "cidr_blocks_ipv4.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.restricted",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.restricted", "cidr_blocks_ipv6.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.restricted",
						"cidr_blocks_ipv6.0", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_restricted_with_directconnectivity,
				Check: resource.ComposeTestCheckFunc(
					// Private Google Access Restricted VIP
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.restricted", "cidr_blocks.#", "4"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.restricted",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.restricted", "cidr_blocks_ipv4.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.restricted",
						"cidr_blocks_ipv4.1", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.restricted", "cidr_blocks_ipv6.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.restricted",
						"cidr_blocks_ipv6.1", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_private,
				Check: resource.ComposeTestCheckFunc(
					// Private Google Access Unrestricted VIP
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.private", "cidr_blocks.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.private",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.private", "cidr_blocks_ipv4.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.private",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.private", "cidr_blocks_ipv6.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.private",
						"cidr_blocks_ipv6.0", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_private_with_directconnectivity,
				Check: resource.ComposeTestCheckFunc(
					// Private Google Access Unrestricted VIP
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.private", "cidr_blocks.#", "4"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.private",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.private", "cidr_blocks_ipv4.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.private",
						"cidr_blocks_ipv4.1", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.private", "cidr_blocks_ipv6.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.private",
						"cidr_blocks_ipv6.1", regexp.MustCompile("^(?:[0-9a-fA-F]{1,4}:){1,2}.*/[0-9]{1,3}$")),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_dns,
				Check: resource.ComposeTestCheckFunc(
					// DNS outbound forwarding
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.dns", "cidr_blocks.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.dns",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.dns", "cidr_blocks_ipv4.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.dns",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.dns", "cidr_blocks_ipv6.#", "0"),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_iap,
				Check: resource.ComposeTestCheckFunc(
					// IAP sources
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.iap", "cidr_blocks.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.iap",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.iap", "cidr_blocks_ipv4.#", "1"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.iap",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.iap", "cidr_blocks_ipv6.#", "0"),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_hc,
				Check: resource.ComposeTestCheckFunc(
					// Modern health checkers
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.hc", "cidr_blocks.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.hc",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.hc", "cidr_blocks_ipv4.#", "2"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.hc",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.hc", "cidr_blocks_ipv6.#", "0"),
				),
			},
			{
				Config: testAccNetblockIpRangesConfig_lhc,
				Check: resource.ComposeTestCheckFunc(
					// Legacy health checkers
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.lhc", "cidr_blocks.#", "3"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.lhc",
						"cidr_blocks.0", regexp.MustCompile("^(?:[0-9a-fA-F./:]{1,4}){1,2}.*/[0-9]{1,3}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.lhc", "cidr_blocks_ipv4.#", "3"),
					resource.TestMatchResourceAttr("data.google_netblock_ip_ranges.lhc",
						"cidr_blocks_ipv4.0", regexp.MustCompile("^(?:[0-9]{1,3}.){3}[0-9]{1,3}/[0-9]{1,2}$")),
					resource.TestCheckResourceAttr("data.google_netblock_ip_ranges.lhc", "cidr_blocks_ipv6.#", "0"),
				),
			},
		},
	})
}

const testAccNetblockIpRangesConfig = `
data "google_netblock_ip_ranges" "cloud" {}
`

const testAccNetblockIpRangesConfig_google = `
data "google_netblock_ip_ranges" "google" {
  range_type = "google-netblocks"
}
`

const testAccNetblockIpRangesConfig_defaultdomains = `
data "google_netblock_ip_ranges" "defaultdomains" {
  range_type = "default-domains-netblocks"
}
`

const testAccNetblockIpRangesConfig_restricted = `
data "google_netblock_ip_ranges" "restricted" {
  range_type = "restricted-googleapis"
}
`

const testAccNetblockIpRangesConfig_restricted_with_directconnectivity = `
data "google_netblock_ip_ranges" "restricted" {
  range_type = "restricted-googleapis-with-directconnectivity"
}
`

const testAccNetblockIpRangesConfig_private = `
data "google_netblock_ip_ranges" "private" {
  range_type = "private-googleapis"
}
`

const testAccNetblockIpRangesConfig_private_with_directconnectivity = `
data "google_netblock_ip_ranges" "private" {
  range_type = "private-googleapis-with-directconnectivity"
}
`

const testAccNetblockIpRangesConfig_dns = `
data "google_netblock_ip_ranges" "dns" {
  range_type = "dns-forwarders"
}
`

const testAccNetblockIpRangesConfig_iap = `
data "google_netblock_ip_ranges" "iap" {
  range_type = "iap-forwarders"
}
`

const testAccNetblockIpRangesConfig_hc = `
data "google_netblock_ip_ranges" "hc" {
  range_type = "health-checkers"
}
`

const testAccNetblockIpRangesConfig_lhc = `
data "google_netblock_ip_ranges" "lhc" {
  range_type = "legacy-health-checkers"
}
`
