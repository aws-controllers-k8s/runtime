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
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
)

const appName = "aws-controllers-k8s"

// NewSession returns a new session object. By default the returned session is
// created using pod IRSA environment variables. If assumeRoleARN is not empty,
// NewSession will call STS::AssumeRole and use the returned credentials to create
// the session.
func (c *serviceController) NewSession(
	region ackv1alpha1.AWSRegion,
	endpointURL *string,
	assumeRoleARN ackv1alpha1.AWSResourceName,
	groupVersionKind schema.GroupVersionKind,
) (*session.Session, error) {
	awsCfg := aws.Config{
		Region:              aws.String(string(region)),
		STSRegionalEndpoint: endpoints.RegionalSTSEndpoint,
	}

	if *endpointURL != "" {
		endpointServiceResolver := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
			if service == c.ServiceEndpointsID {
				return endpoints.ResolvedEndpoint{
					URL: *endpointURL,
				}, nil
			}
			return endpoints.DefaultResolver().EndpointFor(service, region)
		}
		awsCfg.EndpointResolver = endpoints.ResolverFunc(endpointServiceResolver)
	}

	sess, err := session.NewSession(&awsCfg)
	if err != nil {
		return nil, err
	}

	if assumeRoleARN != "" {
		// call STS::AssumeRole
		creds := stscreds.NewCredentials(sess, string(assumeRoleARN))
		// recreate session with the new credentials
		awsCfg.Credentials = creds
		sess, err = session.NewSession(&awsCfg)
		if err != nil {
			return nil, err
		}
	}
	//injecting session handler info
	c.injectUserAgent(&sess.Handlers, groupVersionKind)

	// TODO(jaypipes): Handle throttling
	return sess, nil
}

// injectUserAgent will inject app specific user-agent into awsSDK
func (c *serviceController) injectUserAgent(
	handlers *request.Handlers,
	groupVersionKind schema.GroupVersionKind,
) {
	handlers.Build.PushFrontNamed(request.NamedHandler{
		Name: fmt.Sprintf("%s/user-agent", appName),
		Fn: request.MakeAddToUserAgentHandler(
			appName,
			groupVersionKind.Group+"-"+c.VersionInfo.GitVersion,
			"GitCommit/"+c.VersionInfo.GitCommit,
			"BuildDate/"+c.VersionInfo.BuildDate,
			"CRDKind/"+groupVersionKind.Kind,
			"CRDVersion/"+groupVersionKind.Version),
	})
}
