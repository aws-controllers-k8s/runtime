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

package config

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/jaypipes/envutil"
	flag "github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	flagEnableLeaderElection = "enable-leader-election"
	flagMetricAddr           = "metrics-addr"
	flagEnableDevLogging     = "enable-development-logging"
	flagAWSRegion            = "aws-region"
	flagAWSEndpointURL       = "aws-endpoint-url"
	flagLogLevel             = "log-level"
	flagResourceTags         = "resource-tags"
	flagWatchNamespace       = "watch-namespace"
	flagEnableWebhookServer  = "enable-webhook-server"
	flagWebhookServerAddr    = "webhook-server-addr"
	envVarAWSRegion          = "AWS_REGION"
)

// Config contains configuration otpions for ACK service controllers
type Config struct {
	MetricsAddr              string
	EnableLeaderElection     bool
	EnableDevelopmentLogging bool
	AccountID                string
	Region                   string
	EndpointURL              string
	LogLevel                 string
	ResourceTags             []string
	WatchNamespace           string
	EnableWebhookServer      bool
	WebhookServerAddr        string
}

// BindFlags defines CLI/runtime configuration options
func (cfg *Config) BindFlags() {
	flag.StringVar(
		&cfg.MetricsAddr, flagMetricAddr,
		"0.0.0.0:8080",
		"The address the metric endpoint binds to.",
	)
	flag.BoolVar(
		&cfg.EnableWebhookServer, flagEnableWebhookServer,
		false,
		"Enable webhook server for controller manager.",
	)
	flag.StringVar(
		&cfg.WebhookServerAddr, flagWebhookServerAddr,
		"0.0.0.0:9433",
		"The address the webhook endpoint binds to.",
	)
	flag.BoolVar(
		&cfg.EnableLeaderElection, flagEnableLeaderElection,
		false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.",
	)
	flag.BoolVar(
		&cfg.EnableDevelopmentLogging, flagEnableDevLogging,
		false,
		"Configures the logger to use a Zap development config (encoder=consoleEncoder,logLevel=Debug,stackTraceLevel=Warn, no sampling), "+
			"otherwise a Zap production config will be used (encoder=jsonEncoder,logLevel=Info,stackTraceLevel=Error), sampling).",
	)
	flag.StringVar(
		&cfg.Region, flagAWSRegion,
		envutil.WithDefault(envVarAWSRegion, ""),
		"The AWS Region in which the service controller will create its resources",
	)
	flag.StringVar(
		&cfg.EndpointURL, flagAWSEndpointURL,
		"",
		"The AWS endpoint URL the service controller will use to create its resources. This is an optional"+
			" flag that can be used to override the default behaviour of aws-sdk-go that constructs endpoint URLs"+
			" automatically based on service and region",
	)
	flag.StringVar(
		&cfg.LogLevel, flagLogLevel,
		"info",
		"The log level. Default is info. We use logr interface which only supports info and debug level",
	)
	flag.StringSliceVar(
		&cfg.ResourceTags, flagResourceTags,
		[]string{},
		"Configures the ACK service controller to always set key/value pairs tags on resources that it manages.",
	)
	flag.StringVar(
		&cfg.WatchNamespace, flagWatchNamespace,
		"",
		"Specific namespace the service controller will watch for object creation from CRD. "+
			" By default it will listen to all namespaces",
	)
}

// SetupLogger initializes the logger used in the service controller
func (cfg *Config) SetupLogger() {
	var lvl zapcore.LevelEnabler

	switch cfg.LogLevel {
	case "debug":
		lvl = zapcore.DebugLevel
	default:
		lvl = zapcore.InfoLevel
	}

	zapOptions := zap.Options{
		Development: cfg.EnableDevelopmentLogging,
		Level:       lvl,
	}
	ctrlrt.SetLogger(zap.New(zap.UseFlagOptions(&zapOptions)))
}

// SetAWSAccountID uses sts GetCallerIdentity API to find AWS AccountId and set
// in Config
func (cfg *Config) SetAWSAccountID() error {
	// use sts to find AWS AccountId
	session, err := session.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create session: %v", err)
	}
	client := sts.New(session)
	res, err := client.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("unable to get caller identity: %v", err)
	}
	cfg.AccountID = *res.Account
	return nil
}

// Validate ensures the options are valid
func (cfg *Config) Validate() error {
	if err := cfg.SetAWSAccountID(); err != nil {
		return fmt.Errorf("unable to determine account ID: %v", err)
	}

	if cfg.Region == "" {
		return errors.New("unable to start service controller as AWS region is missing. Please pass --aws-region flag or set AWS_REGION environment variable")
	}

	if cfg.EndpointURL != "" {
		endpoint, err := url.Parse(cfg.EndpointURL)
		if err != nil || endpoint.Scheme != "https" && endpoint.Host != "" {
			return errors.New("invalid service endpoint. Please refer to " +
				"https://docs.aws.amazon.com/general/latest/gr/aws-service-information.html for more details")
		}
	}

	if cfg.EnableWebhookServer && cfg.WebhookServerAddr == "" {
		return errors.New("empty webhook server address")
	}
	return nil
}
