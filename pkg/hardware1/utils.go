package hardware

// min3 returns minimum of three integers
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// abs returns absolute value of integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// float32Ptr 返回 float32 指针
func float32Ptr(f float64) *float32 {
	val := float32(f)
	return &val
}

// intPtr 返回 int 指针
func intPtr(i int) *int {
	return &i
}

// abs64 返回绝对值
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// min 返回最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// pow 计算幂
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}
