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
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	acktags "github.com/aws-controllers-k8s/runtime/pkg/tags"
	ackutil "github.com/aws-controllers-k8s/runtime/pkg/util"
)

const (
	flagEnableAdoptedResourceReconciler = "enable-adopted-resource-reconciler"
	flagEnableLeaderElection            = "enable-leader-election"
	flagLeaderElectionNamespace         = "leader-election-namespace"
	flagMetricAddr                      = "metrics-addr"
	flagHealthzAddr                     = "healthz-addr"
	flagEnableDevLogging                = "enable-development-logging"
	flagAWSRegion                       = "aws-region"
	flagAWSEndpointURL                  = "aws-endpoint-url"
	flagAWSIdentityEndpointURL          = "aws-identity-endpoint-url"
	flagUnsafeAWSEndpointURLs           = "allow-unsafe-aws-endpoint-urls"
	flagLogLevel                        = "log-level"
	flagResourceTags                    = "resource-tags"
	flagWatchNamespace                  = "watch-namespace"
	flagFieldObjectSelector             = "field-object-selector"
	flagEnableWebhookServer             = "enable-webhook-server"
	flagWebhookServerAddr               = "webhook-server-addr"
	flagDeletionPolicy                  = "deletion-policy"
	flagReconcileDefaultResyncSeconds   = "reconcile-default-resync-seconds"
	flagReconcileResourceResyncSeconds  = "reconcile-resource-resync-seconds"
	flagReconcileDefaultMaxConcurrency  = "reconcile-default-max-concurrent-syncs"
	flagReconcileResourceMaxConcurrency = "reconcile-resource-max-concurrent-syncs"
	flagFeatureGates                    = "feature-gates"
	envVarAWSRegion                     = "AWS_REGION"
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
	MetricsAddr                     string
	HealthzAddr                     string
	EnableLeaderElection            bool
	EnableAdoptedResourceReconciler bool
	LeaderElectionNamespace         string
	EnableDevelopmentLogging        bool
	AccountID                       string
	Region                          string
	IdentityEndpointURL             string
	EndpointURL                     string
	AllowUnsafeEndpointURL          bool
	LogLevel                        string
	ResourceTags                    []string
	WatchNamespace                  string
	FieldObjectSelector             string
	EnableWebhookServer             bool
	WebhookServerAddr               string
	DeletionPolicy                  ackv1alpha1.DeletionPolicy
	ReconcileDefaultResyncSeconds   int
	ReconcileResourceResyncSeconds  []string
	ReconcileDefaultMaxConcurrency  int
	ReconcileResourceMaxConcurrency []string
	// TODO(a-hilaly): migrate to k8s.io/component-base and implement a proper parser for feature gates.
	FeatureGates    featuregate.FeatureGates
	featureGatesRaw string
}

// BindFlags defines CLI/runtime configuration options
func (cfg *Config) BindFlags() {
	flag.StringVar(
		&cfg.MetricsAddr, flagMetricAddr,
		"0.0.0.0:8080",
		"The address the metric endpoint binds to.",
	)
	flag.StringVar(
		&cfg.HealthzAddr, flagHealthzAddr,
		"0.0.0.0:8081",
		"The address the health probe endpoint binds to.",
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
		&cfg.EnableAdoptedResourceReconciler, flagEnableAdoptedResourceReconciler,
		true,
		"Enable the AdoptedResource reconciler.",
	)
	flag.StringVar(
		// In the context of the controller-runtime library, if the LeaderElectionNamespace parametere is not
		//  explicitly set, the library will automatically default its value to the content of the file
		// mounted at /var/run/secrets/kubernetes.io/serviceaccount/namespace.
		// https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/leaderelection/leader_election.go#L112-L127
		//
		// In Kubernetes, when a pod is created, a service account is automatically associated with it,
		// unless explicitly specified otherwise. This service account contains relevant information, such
		// as the namespace in which the pod is deployed. The Kubernetes API server mounts a two files
		// for the service account in the pod's filesystem at /var/run/secrets/kubernetes.io/serviceaccount/token
		// and /var/run/secrets/kubernetes.io/serviceaccount/namespace, respectively.
		// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/serviceaccount/tokens_controller.go#L399-L402
		&cfg.LeaderElectionNamespace, flagLeaderElectionNamespace,
		"",
		"Specific namespace that the controller will utilize to manage the coordination.k8s.io/lease object for leader election."+
			" By default it will try to use the namespace of the service account mounted to the controller pod.",
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
		"A comma-separated list of valid RFC-1123 namespace names to watch for custom resource events. "+
			"If unspecified, the controller watches for events in all namespaces.",
	)
	flag.StringVar(
		&cfg.FieldObjectSelector, flagFieldObjectSelector,
		"",
		"A comma-separated list of valid RFC-1123 field object selectors to filter the objects."+
			" For example, you can use a field selector similar to 'metadata.namespace=ns-ack-test' "+
			" to only watch objects that have the 'metadata.namespace' set to 'ns-ack-test'. "+
			" If unspecified, the controller will not filter the objects.",
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
	flag.IntVar(
		&cfg.ReconcileDefaultMaxConcurrency, flagReconcileDefaultMaxConcurrency,
		1,
		"The default maximum number of concurrent reconciles for a resource reconciler. This value is used if no "+
			"resource-specific override has been specified. Default is 1.",
	)
	flag.StringArrayVar(
		&cfg.ReconcileResourceMaxConcurrency, flagReconcileResourceMaxConcurrency,
		[]string{},
		"A Key/Value list of strings representing the reconcile max concurrency configuration for each resource. This"+
			" configuration maps resource kinds to maximum number of concurrent reconciles. If provided, "+
			" resource-specific max concurrency takes precedence over the default max concurrency.",
	)
	flag.StringVar(
		&cfg.featureGatesRaw, flagFeatureGates,
		"",
		"Feature gates to enable. The format is a comma-separated list of key=value pairs. "+
			"Valid keys are feature names and valid values are 'true' or 'false'."+
			"Available features: "+strings.Join(featuregate.GetDefaultFeatureGates().GetFeatureNames(), ", "),
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
	logger := zap.New(zap.UseFlagOptions(&zapOptions))
	ctrlrt.SetLogger(logger)
	klog.SetLogger(logger)
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
	if cfg.ReconcileDefaultMaxConcurrency < 1 {
		return fmt.Errorf("invalid value for flag '%s': max concurrency default must be greater than 0", flagReconcileDefaultMaxConcurrency)
	}

	featureGatesMap, err := parseFeatureGates(cfg.featureGatesRaw)
	if err != nil {
		return fmt.Errorf("invalid value for flag '%s': %v", flagFeatureGates, err)
	}
	cfg.FeatureGates, err = featuregate.GetFeatureGatesWithOverrides(featureGatesMap)
	if err != nil {
		return fmt.Errorf("error overriding feature gates: %v", err)
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

// validateReconcileConfigResources validates the --reconcile-resource-resync-seconds and
// --reconcile-resource-max-concurrent-syncs flags. It ensures that the resource names provided
// in the flags are valid and managed by the controller.
func (cfg *Config) validateReconcileConfigResources(supportedGVKs []schema.GroupVersionKind) error {
	validResourceNames := []string{}
	for _, gvk := range supportedGVKs {
		validResourceNames = append(validResourceNames, gvk.Kind)
	}
	for _, resourceFlagArgument := range cfg.ReconcileResourceResyncSeconds {
		if err := validateReconcileConfigResource(validResourceNames, resourceFlagArgument); err != nil {
			return fmt.Errorf("invalid value for flag '%s': %v", flagReconcileResourceResyncSeconds, err)
		}
	}
	for _, resourceFlagArgument := range cfg.ReconcileResourceMaxConcurrency {
		if err := validateReconcileConfigResource(validResourceNames, resourceFlagArgument); err != nil {
			return fmt.Errorf("invalid value for flag '%s': %v", flagReconcileResourceMaxConcurrency, err)
		}
	}
	return nil
}

// validateReconcileConfigResource validates a single flag argument of any flag that is used to configure
// resource-specific reconcile settings. It ensures that the resource name is valid and managed by the
// controller, and that the value is a positive integer. If the flag argument is not in the expected format
// or has invalid elements, an error is returned.
func validateReconcileConfigResource(validResourceNames []string, resourceFlagArgument string) error {
	resourceName, _, err := parseReconcileFlagArgument(resourceFlagArgument)
	if err != nil {
		return fmt.Errorf("error parsing flag argument '%v': %v. Expected format: string=number", resourceFlagArgument, err)
	}
	if !ackutil.InStrings(resourceName, validResourceNames) {
		return fmt.Errorf(
			"error parsing flag argument '%v': resource '%v' is not managed by this controller. Expected one of %v",
			resourceFlagArgument, resourceName, strings.Join(validResourceNames, ", "),
		)
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

// GetReconcileResourceMaxConcurrency returns the maximum number of concurrent reconciles for a
// given resource name. If the resource name is not found in the --reconcile-resource-max-concurrent-syncs
// flag, the function returns the default maximum concurrency value.
func (cfg *Config) GetReconcileResourceMaxConcurrency(resourceName string) int {
	for _, resourceMaxConcurrencyFlag := range cfg.ReconcileResourceMaxConcurrency {
		// Parse the resource name and max concurrency from the flag argument
		name, maxConcurrency, _ := parseReconcileFlagArgument(resourceMaxConcurrencyFlag)
		if strings.EqualFold(name, resourceName) {
			return maxConcurrency
		}
	}
	return cfg.ReconcileDefaultMaxConcurrency
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

	value, err := strconv.Atoi(elements[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid value in flag argument: %v", err)
	}
	if value <= 0 {
		return "", 0, fmt.Errorf("invalid value in flag argument: value must be greater than 0")
	}
	return elements[0], value, nil
}

// TODO(itaiatu): Add description
func (cfg *Config) ParseFieldObjectSelectors() (fields.Selector, error) {
	selectors := []fields.Selector{}
	fieldPairs := strings.Split(cfg.FieldObjectSelector, ",")

	for _, pair := range fieldPairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			selectors = append(selectors, fields.OneTermEqualSelector(kv[0], kv[1]))
		}
	}

	// Combine all selectors using AND condition
	if len(selectors) > 0 {
		return fields.AndSelectors(selectors...), nil
	}

	// Default empty selector (matches everything)
	return fields.Everything(), nil
}

// GetWatchNamespaces returns a slice of namespaces to watch for custom resource events.
// If the watchNamespace flag is empty, the function returns nil, which means that the
// controller will watch for events in all namespaces.
// TODO: Add info about LabelSelectorNamespace flag
func (c *Config) GetWatchNamespaces() ([]string, error) {
	return parseWatchNamespaceString(c.WatchNamespace)
}

// parseWatchNamespaceString parses the watchNamespace flag and returns a slice of namespaces
// to watch. The input string is expected to be a comma-separated list of namespaces.
//
// When providing multiple namespaces, the watchNamespace string must not contain
// spaces, empty namespaces, or duplicate namespaces.
func parseWatchNamespaceString(namespace string) ([]string, error) {
	if namespace == "" {
		// This means that the user did not provide a value for the watchNamespace flag.
		// In this case, we will watch all namespaces.
		return nil, nil
	}
	visited := make(map[string]bool)
	namespaces := strings.Split(namespace, ",")
	for _, ns := range namespaces {
		if ns == "" {
			return nil, fmt.Errorf("invalid namespace: empty namespace")
		}
		if _, ok := visited[ns]; ok {
			return nil, fmt.Errorf("duplicate namespace '%s'", ns)
		}
		// Settling on the same validation rules as k8s.io/apimachinery/pkg/apis/meta/v1/validation.go
		// for namespace names. prefix=false means that trailing dashes are not allowed. Trailing dashes
		// are allowed when the namespace name is used as part of generation.
		if validationErrorStrings := apimachineryvalidation.ValidateNamespaceName(ns, false); len(validationErrorStrings) > 0 {
			return nil, fmt.Errorf("invalid namespace '%s': %v", ns, validationErrorStrings)
		}
		visited[ns] = true
	}
	return namespaces, nil
}

// parseFeatureGates converts a raw string of feature gate settings into a FeatureGates structure.
//
// The input string should be in the format "feature1=bool,feature2=bool,...".
// For example: "MyFeature=true,AnotherFeature=false"
//
// This function:
// - Parses the input string into individual feature gate settings
// - Validates the format of each setting
// - Converts the boolean values
// - Applies these settings as overrides to the default feature gates
func parseFeatureGates(featureGatesRaw string) (map[string]bool, error) {
	featureGatesRaw = strings.TrimSpace(featureGatesRaw)
	if featureGatesRaw == "" {
		return nil, nil
	}

	featureGatesMap := map[string]bool{}
	for _, featureGate := range strings.Split(featureGatesRaw, ",") {
		featureGateKV := strings.SplitN(featureGate, "=", 2)
		if len(featureGateKV) != 2 {
			return nil, fmt.Errorf("invalid feature gate format: %s", featureGate)
		}

		featureName := strings.TrimSpace(featureGateKV[0])
		if featureName == "" {
			return nil, fmt.Errorf("invalid feature gate name: %s", featureGate)
		}

		featureValue := strings.TrimSpace(featureGateKV[1])
		featureEnabled, err := strconv.ParseBool(featureValue)
		if err != nil {
			return nil, fmt.Errorf("invalid feature gate value for %s: %s", featureName, featureValue)
		}

		featureGatesMap[featureName] = featureEnabled
	}

	return featureGatesMap, nil
}
