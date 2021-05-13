package compare_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws-controllers-k8s/runtime/pkg/compare"
)

func TestNilDifference(t *testing.T) {
	require := require.New(t)

	sampleString := ""
	var nullPtr *string
	nonNullPtr := &sampleString
	var nullMap map[string]string
	nonNullMap := make(map[string]string)
	var nullArray []string
	nonNullArray := make([]string, 0)
	var nullChan chan int
	nonNullChan := make(chan int, 0)

	require.False(compare.HasNilDifference(nil, nil))
	require.False(compare.HasNilDifference("a", "b"))
	require.True(compare.HasNilDifference(nil, "b"))
	require.True(compare.HasNilDifference("a", nil))

	//Pointer
	require.False(compare.HasNilDifference(nullPtr, nil))
	require.True(compare.HasNilDifference(nullPtr, nonNullPtr))

	//Map
	require.False(compare.HasNilDifference(nullMap, nil))
	require.True(compare.HasNilDifference(nullMap, nonNullMap))

	//Array or Slice
	require.False(compare.HasNilDifference(nullArray, nil))
	require.True(compare.HasNilDifference(nullArray, nonNullArray))

	//Chan
	require.False(compare.HasNilDifference(nullChan, nil))
	require.True(compare.HasNilDifference(nullChan, nonNullChan))

}

