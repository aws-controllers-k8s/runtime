package types_test

import (
	"testing"
	"time"

	types "github.com/aws-controllers-k8s/runtime/pkg/types"
)

func TestExponential_GetBackoff(t *testing.T) {
	type fields struct {
		Initial  time.Duration
		Factor   float64
		MaxDelay time.Duration
	}
	type args struct {
		numAttempt int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   time.Duration
	}{
		{
			name:   "Default Values,Zero Attempt",
			fields: fields{},
			args:   args{0},
			want:   time.Duration(0) * time.Second,
		},
		{
			name:   "Default Values,First Attempt",
			fields: fields{},
			args:   args{1},
			want:   time.Duration(1) * time.Second,
		},
		{
			name:   "Default Values,Fifth Attempt",
			fields: fields{},
			args:   args{5},
			want:   time.Duration(16) * time.Second,
		},
		{
			name:   "Only Min,Zero Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second},
			args:   args{0},
			want:   time.Duration(5) * time.Second,
		},
		{
			name:   "Only Min,First Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second},
			args:   args{1},
			want:   time.Duration(6) * time.Second,
		},
		{
			name:   "Only Min,Fifth Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second},
			args:   args{5},
			want:   time.Duration(21) * time.Second,
		},
		{
			name:   "Only Max,Zero Attempt",
			fields: fields{MaxDelay: time.Duration(10) * time.Second},
			args:   args{0},
			want:   time.Duration(0) * time.Second,
		},
		{
			name:   "Only Max,First Attempt",
			fields: fields{MaxDelay: time.Duration(10) * time.Second},
			args:   args{1},
			want:   time.Duration(1) * time.Second,
		},
		{
			name:   "Only Max,Fifth Attempt",
			fields: fields{MaxDelay: time.Duration(10) * time.Second},
			args:   args{5},
			want:   time.Duration(10) * time.Second,
		},
		{
			name:   "Min & Max,Zero Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second, MaxDelay: time.Duration(15) * time.Second},
			args:   args{0},
			want:   time.Duration(5) * time.Second,
		},
		{
			name:   "Min & Max,First Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second, MaxDelay: time.Duration(15) * time.Second},
			args:   args{1},
			want:   time.Duration(6) * time.Second,
		},
		{
			name:   "Min & Max,Fifth Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second, MaxDelay: time.Duration(15) * time.Second},
			args:   args{5},
			want:   time.Duration(15) * time.Second,
		},
		{
			name:   "Min & Max,Factor 3, Zero Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second, Factor: 3, MaxDelay: time.Duration(15) * time.Second},
			args:   args{0},
			want:   time.Duration(5) * time.Second,
		},
		{
			name:   "Min & Max,Factor 3, Second Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second, Factor: 3, MaxDelay: time.Duration(15) * time.Second},
			args:   args{2},
			want:   time.Duration(8) * time.Second,
		},
		{
			name:   "Min & Max,Factor 3, Fifth Attempt",
			fields: fields{Initial: time.Duration(5) * time.Second, Factor: 3, MaxDelay: time.Duration(15) * time.Second},
			args:   args{5},
			want:   time.Duration(15) * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &types.Exponential{
				Initial:  tt.fields.Initial,
				Factor:   tt.fields.Factor,
				MaxDelay: tt.fields.MaxDelay,
			}
			if got := l.GetBackoff(tt.args.numAttempt); got != tt.want {
				t.Errorf("GetBackoff() = %v, want %v", got, tt.want)
			}
		})
	}
}
