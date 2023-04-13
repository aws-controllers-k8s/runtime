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
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/jaypipes/envutil"
	flag "github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
	ackutil "github.com/aws-controllers-k8s/runtime/pkg/util"
)

const (
	flagEnableLeaderElection           = "enable-leader-election"
	flagMetricAddr                     = "metrics-addr"
	flagEnableDevLogging               = "enable-development-logging"
	flagAWSRegion                      = "aws-region"
	flagAWSEndpointURL                 = "aws-endpoint-url"
	flagAWSIdentityEndpointURL         = "aws-identity-endpoint-url"
	flagUnsafeAWSEndpointURLs          = "allow-unsafe-aws-endpoint-urls"
	flagLogLevel                       = "log-level"
	flagResourceTags                   = "resource-tags"
	flagWatchNamespace                 = "watch-namespace"
	flagEnableWebhookServer            = "enable-webhook-server"
	flagWebhookServerAddr              = "webhook-server-addr"
	flagDeletionPolicy                 = "deletion-policy"
	flagReconcileDefaultResyncSeconds  = "reconcile-default-resync-seconds"
	flagReconcileResourceResyncSeconds = "reconcile-resource-resync-seconds"
	envVarAWSRegion                    = "AWS_REGION"
)

var (
	defaultResourceTags = []string{
		fmt.Sprintf("services.k8s.aws/controller-version=%s-%s",
			acktags.ServiceAliasTagFormat,
			acktags.ControllerVersionTagFormat,
		),
		fmt.Sprintf("services.k8s.aws/namespace=%s",
			acktags.NamespaceTagFormat,
		),
	}
	defaultLogLevel = zapcore.InfoLevel
)

// Config contains configuration options for ACK service controllers
type Config struct {
	MetricsAddr                    string
	EnableLeaderElection           bool
	EnableDevelopmentLogging       bool
	AccountID                      string
	Region                         string
	IdentityEndpointURL            string
	EndpointURL                    string
	AllowUnsafeEndpointURL         bool
	LogLevel                       string
	ResourceTags                   []string
	WatchNamespace                 string
	EnableWebhookServer            bool
	WebhookServerAddr              string
	DeletionPolicy                 ackv1alpha1.DeletionPolicy
	ReconcileDefaultResyncSeconds  int
	ReconcileResourceResyncSeconds []string
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
		&cfg.IdentityEndpointURL, flagAWSIdentityEndpointURL,
		"",
		"The AWS endpoint URL the service controller will use to gather information from STS. This is an optional"+
			" flag that can be used to override the default behaviour of aws-sdk-go that constructs endpoint URLs"+
			" automatically based on service and region",
	)
	flag.BoolVar(
		&cfg.AllowUnsafeEndpointURL, flagUnsafeAWSEndpointURLs,
		false,
		"Allow an unsafe AWS endpoint URL over http",
	)
	flag.StringVar(
		&cfg.LogLevel, flagLogLevel,
		"info",
		"The log level. The default is info. The options are: debug, info, warn, error, dpanic, panic, fatal",
	)
	flag.StringSliceVar(
		&cfg.ResourceTags, flagResourceTags,
		defaultResourceTags,
		"Configures the ACK service controller to always set key/value pairs tags on resources that it manages.",
	)
	flag.StringVar(
		&cfg.WatchNamespace, flagWatchNamespace,
		"",
		"Specific namespace the service controller will watch for object creation from CRD. "+
			" By default it will listen to all namespaces",
	)
	flag.Var(
		&cfg.DeletionPolicy, flagDeletionPolicy,
		"The default deletion policy for all resources managed by the controller",
	)
	flag.IntVar(
		&cfg.ReconcileDefaultResyncSeconds, flagReconcileDefaultResyncSeconds,
		0,
		"The default duration, in seconds, to wait before resyncing desired state of custom resources. "+
			"This value is used if no resource-specific override has been specified. Default is 10 hours.",
	)
	flag.StringArrayVar(
		&cfg.ReconcileResourceResyncSeconds, flagReconcileResourceResyncSeconds,
		[]string{},
		"A Key/Value list of strings representing the reconcile resync configuration for each resource. This"+
			" configuration maps resource kinds to drift remediation periods in seconds. If provided, "+
			" resource-specific resync periods take precedence over the default period.",
	)
}

// SetupLogger initializes the logger used in the service controller
func (cfg *Config) SetupLogger() {
	lvl := defaultLogLevel
	lvl.UnmarshalText([]byte(cfg.LogLevel))

	zapOptions := zap.Options{
		Development: cfg.EnableDevelopmentLogging,
		Level:       lvl,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	ctrlrt.SetLogger(zap.New(zap.UseFlagOptions(&zapOptions)))
}

// SetAWSAccountID uses sts GetCallerIdentity API to find AWS AccountId and set
// in Config
func (cfg *Config) SetAWSAccountID() error {

	awsCfg := aws.Config{}
	if cfg.IdentityEndpointURL != "" {
		awsCfg.Endpoint = aws.String(cfg.IdentityEndpointURL)
	}

	// use sts to find AWS AccountId
	session, err := session.NewSession(&awsCfg)
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
func (cfg *Config) Validate(options ...Option) error {
	merged := mergeOptions(options)
	if len(merged.gvks) > 0 {
		err := cfg.validateReconcileConfigResources(merged.gvks)
		if err != nil {
			return fmt.Errorf("invalid value for flag '%s': %v", flagReconcileResourceResyncSeconds, err)
		}
	}

	if cfg.Region == "" {
		return errors.New("unable to start service controller as AWS region is missing. Please pass --aws-region flag or set AWS_REGION environment variable")
	}

	if cfg.EndpointURL != "" {
		serviceEndpoint, err := url.Parse(cfg.EndpointURL)
		if err != nil {
			return errors.New("invalid service endpoint. Please refer to " +
				"https://docs.aws.amazon.com/general/latest/gr/aws-service-information.html for more details")
		}

		// Throw an error if URL is unsafe and config.AllowUnsafeEndpointURL is not set accordingly
		if err := cfg.checkUnsafeEndpoint(serviceEndpoint); err != nil {
			return err
		}
	}

	if cfg.IdentityEndpointURL != "" {
		identityEndpoint, err := url.Parse(cfg.IdentityEndpointURL)
		if err != nil {
			return errors.New("invalid identity endpoint. Please refer to " +
				"https://docs.aws.amazon.com/general/latest/gr/aws-service-information.html for more details")
		}

		// Throw an error if URL is unsafe and config.AllowUnsafeEndpointURL is not set accordingly
		if err := cfg.checkUnsafeEndpoint(identityEndpoint); err != nil {
			return err
		}
	}

	if err := cfg.SetAWSAccountID(); err != nil {
		return fmt.Errorf("unable to determine account ID: %v", err)
	}

	if cfg.EnableWebhookServer && cfg.WebhookServerAddr == "" {
		return errors.New("empty webhook server address")
	}

	if cfg.DeletionPolicy == "" {
		cfg.DeletionPolicy = ackv1alpha1.DeletionPolicyDelete
	}

	if cfg.ReconcileDefaultResyncSeconds < 0 {
		return fmt.Errorf("invalid value for flag '%s': resync seconds default must be greater than 0", flagReconcileDefaultResyncSeconds)
	}
	return nil
}

func (cfg *Config) checkUnsafeEndpoint(endpoint *url.URL) error {
	if !cfg.AllowUnsafeEndpointURL {
		if endpoint.Scheme != "https" && endpoint.Host != "" {
			return errors.New("using an unsafe endpoint is not allowed. Please review the controller configuration")
		}
	}
	return nil
}

// validateReconcileConfigResources validates the --reconcile-resource-resync-seconds flag
// by checking the resource names and their corresponding duration.
func (cfg *Config) validateReconcileConfigResources(supportedGVKs []schema.GroupVersionKind) error {
	validResourceNames := []string{}
	for _, gvk := range supportedGVKs {
		validResourceNames = append(validResourceNames, gvk.Kind)
	}
	for _, resourceResyncSecondsFlag := range cfg.ReconcileResourceResyncSeconds {
		resourceName, _, err := parseReconcileFlagArgument(resourceResyncSecondsFlag)
		if err != nil {
			return fmt.Errorf("error parsing flag argument '%v': %v. Expected format: resource=seconds", resourceResyncSecondsFlag, err)
		}
		if !ackutil.InStrings(resourceName, validResourceNames) {
			return fmt.Errorf(
				"error parsing flag argument '%v': resource '%v' is not managed by this controller. Expected one of %v",
				resourceResyncSecondsFlag, resourceName, strings.Join(validResourceNames, ", "),
			)
		}
	}
	return nil
}

// ParseReconcileResourceResyncSeconds parses the values of the --reconcile-resource-resync-seconds
// flag and returns a map that maps resource names to resync periods.
// The flag arguments are expected to have the format "resource=seconds", where "resource" is the
// name of the resource and "seconds" is the number of seconds that the reconciler should wait before
// reconciling the resource again.
func (cfg *Config) ParseReconcileResourceResyncSeconds() (map[string]time.Duration, error) {
	resourceResyncPeriods := make(map[string]time.Duration, len(cfg.ReconcileResourceResyncSeconds))
	for _, resourceResyncSecondsFlag := range cfg.ReconcileResourceResyncSeconds {
		// Parse the resource name and resync period from the flag argument
		resourceName, resyncSeconds, _ := parseReconcileFlagArgument(resourceResyncSecondsFlag)
		resourceResyncPeriods[strings.ToLower(resourceName)] = time.Duration(resyncSeconds)
	}
	return resourceResyncPeriods, nil
}

// parseReconcileFlagArgument parses a flag argument of the form "key=value" into
// its individual elements. The key must be a non-empty string and the value must be
// a non-empty positive integer. If the flag argument is not in the expected format
// or has invalid elements, an error is returned.
//
// The function returns the parsed key and value as separate elements.
func parseReconcileFlagArgument(flagArgument string) (string, int, error) {
	delimiter := "="
	elements := strings.Split(flagArgument, delimiter)
	if len(elements) != 2 {
		return "", 0, fmt.Errorf("invalid flag argument format: expected key=value")
	}
	if elements[0] == "" {
		return "", 0, fmt.Errorf("missing key in flag argument")
	}
	if elements[1] == "" {
		return "", 0, fmt.Errorf("missing value in flag argument")
	}

	resyncSeconds, err := strconv.Atoi(elements[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid value in flag argument: %v", err)
	}
	if resyncSeconds < 0 {
		return "", 0, fmt.Errorf("invalid value in flag argument: expected non-negative integer, got %d", resyncSeconds)
	}
	return elements[0], resyncSeconds, nil
}
