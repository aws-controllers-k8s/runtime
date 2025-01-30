// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

const appName = "aws-controllers-k8s"

func (c *serviceController) NewAWSConfig(
	ctx context.Context,
	region ackv1alpha1.AWSRegion,
	endpointURL *string,
	roleARN ackv1alpha1.AWSResourceName,
	groupVersionKind schema.GroupVersionKind,
) (aws.Config, error) {

	val := formatUserAgent(
		appName,
		groupVersionKind.Group+"-"+c.VersionInfo.GitVersion,
		"GitCommit/"+c.VersionInfo.GitCommit,
		"BuildDate/"+c.VersionInfo.BuildDate,
		"CRDKind/"+groupVersionKind.Kind,
		"CRDVersion/"+groupVersionKind.Version,
	)

	userAgentAppender := func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc(fmt.Sprintf("%s/user-agent", appName), func(
			ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler,
		) (
			middleware.BuildOutput, middleware.Metadata, error,
		) {
			switch v := in.Request.(type) {
			case *smithyhttp.Request:
				v.Header.Add(appName, val)
			}
			return next.HandleBuild(ctx, in)

		}), middleware.Before)
	}

	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(string(region)),
		config.WithAPIOptions([]func(*middleware.Stack) error{
			userAgentAppender,
		}),
	)
	if err != nil {
		return awsCfg, err
	}

	if endpointURL != nil && *endpointURL != "" {
		awsCfg.BaseEndpoint = endpointURL
	}

	if roleARN != "" {
		client := sts.NewFromConfig(awsCfg)
		creds := stscreds.NewAssumeRoleProvider(client, string(roleARN))
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}
	return awsCfg, nil
}

func formatUserAgent(name, version string, extra ...string) string {
	ua := fmt.Sprintf("%s/%s", name, version)
	if len(extra) > 0 {
		ua += fmt.Sprintf(" (%s)", strings.Join(extra, "; "))
	}
	return ua
}
