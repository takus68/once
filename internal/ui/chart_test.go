package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChartView(t *testing.T) {
	// 80 data points = 40 chars wide (2 points per character)
	data := []float64{
		10, 15, 25, 20, 15, 25, 40, 35, 35, 40,
		50, 45, 45, 50, 30, 40, 55, 50, 60, 55,
		45, 55, 70, 65, 65, 70, 80, 75, 75, 80,
		90, 85, 85, 90, 70, 80, 95, 90, 100, 95,
		85, 90, 75, 80, 90, 85, 80, 85, 70, 75,
		85, 80, 75, 80, 65, 70, 80, 75, 70, 75,
		60, 65, 75, 70, 65, 70, 55, 60, 70, 65,
		60, 65, 50, 55, 65, 60, 55, 60, 45, 50,
	}

	chart := NewChart("Test", UnitCount)
	output := chart.View(data, 40, 6, NewChartScale(UnitCount, 100))

	assert.NotEmpty(t, output)
}

func TestSlidingSum(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	result := SlidingSum(data, 2)

	// Each output[i] = sum of data[i-1:i+1] (backward looking)
	// [0]: 1 (only 1 element available)
	// [1]: 1+2 = 3
	// [2]: 2+3 = 5
	// [3]: 3+4 = 7
	// [4]: 4+5 = 9
	assert.Equal(t, []float64{1, 3, 5, 7, 9}, result)
}

func TestSlidingSumLargeWindow(t *testing.T) {
	data := []float64{1, 2, 3}
	result := SlidingSum(data, 5)

	// Window larger than available data, backward looking
	// [0]: 1 (only 1 available)
	// [1]: 1+2 = 3
	// [2]: 1+2+3 = 6
	assert.Equal(t, []float64{1, 3, 6}, result)
}

func TestSlidingSumEmpty(t *testing.T) {
	result := SlidingSum([]float64{}, 3)
	assert.Empty(t, result)
}

func TestSlidingSumWindowOne(t *testing.T) {
	data := []float64{10, 20, 30}
	result := SlidingSum(data, 1)
	assert.Equal(t, data, result)
}

func TestUnitTypeFormat(t *testing.T) {
	assert.Equal(t, "50%", UnitPercent.Format(50))
	assert.Equal(t, "100%", UnitPercent.Format(99.9))

	assert.Equal(t, "512B", UnitBytes.Format(512))
	assert.Equal(t, "1K", UnitBytes.Format(1024))
	assert.Equal(t, "128M", UnitBytes.Format(128*1024*1024))
	assert.Equal(t, "1.5G", UnitBytes.Format(1.5*1024*1024*1024))

	assert.Equal(t, "500", UnitCount.Format(500))
	assert.Equal(t, "1.5K", UnitCount.Format(1500))
	assert.Equal(t, "2.5M", UnitCount.Format(2500000))
}
