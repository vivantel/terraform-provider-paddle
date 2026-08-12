package provider

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// checkListContainsID confirms want appears as the "id" of some element in
// dataSourceName's listAttr list — shared by every plural data source's
// acceptance test, since a filtered list can legitimately return more than
// the one fixture record a test provisioned (the sandbox account may have
// other matching records from other test runs), unlike a singular data
// source's exact-index checks.
func checkListContainsID(dataSourceName, listAttr, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", dataSourceName)
		}
		countStr, ok := rs.Primary.Attributes[listAttr+".#"]
		if !ok {
			return fmt.Errorf("%s.%s.# not set in state", dataSourceName, listAttr)
		}
		count, err := strconv.Atoi(countStr)
		if err != nil {
			return fmt.Errorf("%s.%s.# = %q is not a number: %w", dataSourceName, listAttr, countStr, err)
		}
		for i := 0; i < count; i++ {
			if rs.Primary.Attributes[fmt.Sprintf("%s.%d.id", listAttr, i)] == want {
				return nil
			}
		}
		return fmt.Errorf("%s.%s (%d entries) does not contain id %q", dataSourceName, listAttr, count, want)
	}
}

// checkListAttrsSet confirms every attr in attrs is set (non-empty) on
// element index 0 of dataSourceName's listAttr list — a lighter-weight
// shape check for plural data sources whose fixture can't guarantee a
// specific match beyond "at least one element with these fields present."
func checkListAttrsSet(dataSourceName, listAttr string, attrs ...string) resource.TestCheckFunc {
	checks := make([]resource.TestCheckFunc, 0, len(attrs))
	for _, a := range attrs {
		checks = append(checks, resource.TestCheckResourceAttrSet(dataSourceName, fmt.Sprintf("%s.0.%s", listAttr, a)))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}
