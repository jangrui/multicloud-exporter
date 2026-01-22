package metrics

// RecordFourDimensionAccountStatus 记录账号状态
func RecordFourDimensionAccountStatus(accountID, provider, status string) {
	AccountStatusTotal.WithLabelValues(accountID, provider, status).Set(1)
}

// RecordFourDimensionAccountSkip 记录账号跳过
func RecordFourDimensionAccountSkip(accountID, provider, reason string) {
	AccountSkipTotal.WithLabelValues(accountID, provider, reason).Inc()
}

// RecordFourDimensionAccountDegraded 记录账号降级
func RecordFourDimensionAccountDegraded(accountID, provider, reason string) {
	AccountDegradedTotal.WithLabelValues(accountID, provider, reason).Inc()
}

// RecordFourDimensionAccountStatusChange 记录账号状态变更
func RecordFourDimensionAccountStatusChange(accountID, provider, oldStatus, newStatus, reason string) {
	AccountStatusChange.WithLabelValues(accountID, provider, oldStatus, newStatus, reason).Inc()
}

// RecordFourDimensionProductStatus 记录产品状态
func RecordFourDimensionProductStatus(accountID, productID, status string) {
	ProductStatusTotal.WithLabelValues(accountID, productID, status).Set(1)
}

// RecordFourDimensionProductSkip 记录产品跳过
func RecordFourDimensionProductSkip(accountID, productID, reason string) {
	ProductSkipTotal.WithLabelValues(accountID, productID, reason).Inc()
}

// RecordFourDimensionProductDegraded 记录产品降级
func RecordFourDimensionProductDegraded(accountID, productID, reason string) {
	ProductDegradedTotal.WithLabelValues(accountID, productID, reason).Inc()
}

// RecordFourDimensionRegionStatus 记录区域状态
func RecordFourDimensionRegionStatus(accountID, productID, region, status string) {
	RegionStatusTotal.WithLabelValues(accountID, productID, region, status).Set(1)
}

// RecordFourDimensionRegionSkip 记录区域跳过
func RecordFourDimensionRegionSkip(accountID, productID, region, reason string) {
	RegionSkipTotal.WithLabelValues(accountID, productID, region, reason).Inc()
}

// RecordFourDimensionRegionDegraded 记录区域降级
func RecordFourDimensionRegionDegraded(accountID, productID, region, reason string) {
	RegionDegradedTotal.WithLabelValues(accountID, productID, region, reason).Inc()
}

// RecordFourDimensionResourceStatus 记录资源状态
func RecordFourDimensionResourceStatus(accountID, productID, region, resourceID, status string) {
	ResourceStatusTotal.WithLabelValues(accountID, productID, region, resourceID, status).Set(1)
}

// RecordFourDimensionResourceSkip 记录资源跳过
func RecordFourDimensionResourceSkip(accountID, productID, region, resourceID, reason string) {
	ResourceSkipTotal.WithLabelValues(accountID, productID, region, resourceID, reason).Inc()
}

// RecordFourDimensionResourceDegraded 记录资源降级
func RecordFourDimensionResourceDegraded(accountID, productID, region, resourceID, reason string) {
	ResourceDegradedTotal.WithLabelValues(accountID, productID, region, resourceID, reason).Inc()
}
