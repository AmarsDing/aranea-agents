package usage

import "time"

var NormalizeTokenUsageEventForInsert = normalizeTokenUsageEventForInsert
var CsvEscape = csvEscape
var FormatUsageEventsCSV = formatUsageEventsCSV
var UsageCostMicro = usageCostMicro
var ApplyPricingUSDToEvent = applyPricingUSDToEvent

func NormalizeQuery(u *Usecase, query Query, now time.Time) Query {
	return u.normalizeQuery(query, now)
}

func AlertRecentlyFired(u *Usecase, a BudgetAlert, now time.Time) bool {
	return u.alertRecentlyFired(a, now)
}

func MarkAlertFired(u *Usecase, id string, now time.Time) {
	u.markAlertFired(id, now)
}

func SetUsecaseNow(u *Usecase, now func() time.Time) {
	u.now = now
}
