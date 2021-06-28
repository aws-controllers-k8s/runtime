package util_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aws-controllers-k8s/runtime/pkg/util"
)

func TestGetHostPort(t *testing.T) {
	assert := assert.New(t)
	type args struct {
		address string
	}
	testCases := []struct {
		name     string
		args     args
		wantErr  bool
		wantHost string
		wantPort int
	}{
		{
			name: "empty string",
			args: args{
				address: "",
			},
			wantErr: true,
		},
		{
			name: "malformed address",
			args: args{
				address: "host::1",
			},
			wantErr: true,
		},
		{
			name: "port only",
			args: args{
				address: ":500",
			},
			wantErr:  false,
			wantHost: "",
			wantPort: 500,
		},
		{
			name: "host only - port should be specified",
			args: args{
				address: "localhost",
			},
			wantErr: true,
		},
		{
			name: "ipv4",
			args: args{
				address: "0.0.0.0:80",
			},
			wantErr:  false,
			wantHost: "0.0.0.0",
			wantPort: 80,
		},
		{
			name: "ipv6",
			args: args{
				address: "[0::0]:443",
			},
			wantErr:  false,
			wantHost: "0::0",
			wantPort: 443,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, err := util.GetHostPort(
				tc.args.address,
			)
			if (err != nil) != tc.wantErr {
				assert.Fail(fmt.Sprintf("GetHostPort(%s) error = %v, wantErr %v", tc.name, err, tc.wantErr))
			} else if !tc.wantErr {
				assert.Equal(tc.wantHost, host)
				assert.Equal(tc.wantPort, port)
			}
		})
	}
}
