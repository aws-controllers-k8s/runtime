package types_test

import (
	"testing"

	types "github.com/aws-controllers-k8s/runtime/pkg/types"
)

func TestLateInitializationRetryConfig_GetExponentialBackoffSeconds(t *testing.T) {
	type fields struct {
		MinBackoffSeconds int
		MaxBackoffSeconds int
	}
	type args struct {
		numAttempt int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   int
	}{
		{
			name:   "Default Values,Zero Attempt",
			fields: fields{},
			args:   args{0},
			want:   0,
		},
		{
			name:   "Default Values,First Attempt",
			fields: fields{},
			args:   args{1},
			want:   1,
		},
		{
			name:   "Default Values,Fifth Attempt",
			fields: fields{},
			args:   args{5},
			want:   16,
		},
		{
			name:   "Only Min,Zero Attempt",
			fields: fields{MinBackoffSeconds: 5},
			args:   args{0},
			want:   5,
		},
		{
			name:   "Only Min,First Attempt",
			fields: fields{MinBackoffSeconds: 5},
			args:   args{1},
			want:   6,
		},
		{
			name:   "Only Min,Fifth Attempt",
			fields: fields{MinBackoffSeconds: 5},
			args:   args{5},
			want:   21,
		},
		{
			name:   "Only Max,Zero Attempt",
			fields: fields{MaxBackoffSeconds: 10},
			args:   args{0},
			want:   0,
		},
		{
			name:   "Only Max,First Attempt",
			fields: fields{MaxBackoffSeconds: 10},
			args:   args{1},
			want:   1,
		},
		{
			name:   "Only Max,Fifth Attempt",
			fields: fields{MaxBackoffSeconds: 10},
			args:   args{5},
			want:   10,
		},
		{
			name:   "Min & Max,Zero Attempt",
			fields: fields{MinBackoffSeconds: 5, MaxBackoffSeconds: 15},
			args:   args{0},
			want:   5,
		},
		{
			name:   "Min & Max,First Attempt",
			fields: fields{MinBackoffSeconds: 5, MaxBackoffSeconds: 15},
			args:   args{1},
			want:   6,
		},
		{
			name:   "Min & Max,Fifth Attempt",
			fields: fields{MinBackoffSeconds: 5, MaxBackoffSeconds: 15},
			args:   args{5},
			want:   15,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &types.LateInitializationRetryConfig{
				MinBackoffSeconds: tt.fields.MinBackoffSeconds,
				MaxBackoffSeconds: tt.fields.MaxBackoffSeconds,
			}
			if got := l.GetExponentialBackoffSeconds(tt.args.numAttempt); got != tt.want {
				t.Errorf("GetExponentialBackoffSeconds() = %v, want %v", got, tt.want)
			}
		})
	}
}
