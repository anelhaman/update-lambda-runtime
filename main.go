package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lamtypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"
)

type AWSOpts struct {
	Profile       string
	Regions       []string
	FunctionName  string
	All           bool
	SourceRuntime string
	TargetRuntime string
	Timeout       time.Duration
	PollEvery     time.Duration
	ShowProfile   bool // default false; output focuses on AccountID

}

func main() {
	opts := &AWSOpts{
		SourceRuntime: "python3.9",
		TargetRuntime: "python3.12",
		Timeout:       5 * time.Minute,
		PollEvery:     5 * time.Second,
		ShowProfile:   false,
	}

	rootCmd := &cobra.Command{
		Use:   "update-lambda-runtime",
		Short: "Manage AWS Lambda runtimes across accounts/regions",
	}

	rootCmd.PersistentFlags().StringVar(&opts.Profile, "profile", "", "AWS CLI profile (required)")
	rootCmd.PersistentFlags().StringSliceVar(&opts.Regions, "regions", nil, "Comma or multiple --regions (required)")
	rootCmd.PersistentFlags().StringVar(&opts.FunctionName, "function", "", "Lambda function name (if not using --all)")
	rootCmd.PersistentFlags().BoolVar(&opts.All, "all", false, "Process all functions in region(s)")
	rootCmd.PersistentFlags().StringVar(&opts.SourceRuntime, "source-runtime", opts.SourceRuntime, "Only update from this runtime")
	rootCmd.PersistentFlags().StringVar(&opts.TargetRuntime, "target-runtime", opts.TargetRuntime, "Update to this runtime")
	rootCmd.PersistentFlags().DurationVar(&opts.Timeout, "wait-timeout", opts.Timeout, "Max time to wait for update")
	rootCmd.PersistentFlags().DurationVar(&opts.PollEvery, "wait-interval", opts.PollEvery, "Polling interval during update")
	rootCmd.PersistentFlags().BoolVar(&opts.ShowProfile, "show-profile", opts.ShowProfile, "Also print profile column")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List Lambda functions and runtimes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(opts)
		},
	}

	bumpCmd := &cobra.Command{
		Use:   "bump",
		Short: fmt.Sprintf("Update Lambda runtime from %s to %s", opts.SourceRuntime, opts.TargetRuntime),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBump(opts)
		},
	}

	rootCmd.AddCommand(listCmd, bumpCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

// --- core flows ---
func runList(opts *AWSOpts) error {
	if err := validateCommon(opts); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	printHeader(tw, opts.ShowProfile)

	acctID, err := resolveAccountID(opts.Profile)
	if err != nil {
		return fmt.Errorf("resolve account id: %w", err)
	}

	for _, region := range opts.Regions {
		cli, err := lambdaClient(region, opts.Profile)
		if err != nil {
			return err
		}
		if opts.FunctionName != "" {
			rt, _ := getRuntime(cli, opts.FunctionName)
			printRow(tw, acctID, opts.Profile, region, opts.FunctionName, rt, opts.ShowProfile)
		} else {
			funcs, _ := listAllFunctions(cli)
			for _, f := range funcs {
				printRow(tw, acctID, opts.Profile, region, aws.ToString(f.FunctionName), string(f.Runtime), opts.ShowProfile)
			}
		}
	}
	tw.Flush()
	return nil
}

func runBump(opts *AWSOpts) error {
	if err := validateCommon(opts); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	printHeader(tw, opts.ShowProfile)

	acctID, err := resolveAccountID(opts.Profile)
	if err != nil {
		return fmt.Errorf("resolve account id: %w", err)
	}

	for _, region := range opts.Regions {
		cli, err := lambdaClient(region, opts.Profile)
		if err != nil {
			return err
		}
		if opts.FunctionName != "" {
			rt, _ := getRuntime(cli, opts.FunctionName)
			printRow(tw, acctID, opts.Profile, region, opts.FunctionName, rt, opts.ShowProfile)
			if rt == opts.SourceRuntime {
				updateAndWait(cli, opts.FunctionName, opts.TargetRuntime, opts.Timeout, opts.PollEvery)
			}
		} else {
			funcs, _ := listAllFunctions(cli)
			for _, f := range funcs {
				fn := aws.ToString(f.FunctionName)
				rt := string(f.Runtime)
				printRow(tw, acctID, opts.Profile, region, fn, rt, opts.ShowProfile)
				if rt == opts.SourceRuntime {
					updateAndWait(cli, fn, opts.TargetRuntime, opts.Timeout, opts.PollEvery)
				}
			}
		}
	}
	tw.Flush()
	return nil
}

func validateCommon(opts *AWSOpts) error {
	if opts.Profile == "" || len(opts.Regions) == 0 {
		return fmt.Errorf("--profile and --regions are required")
	}
	if opts.FunctionName == "" && !opts.All {
		return fmt.Errorf("specify --function or --all")
	}
	return nil
}

func lambdaClient(region, profile string) (*lambda.Client, error) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, err
	}
	return lambda.NewFromConfig(cfg), nil
}

func stsClient(profile string) (*sts.Client, error) {
	ctx := context.Background()
	// Region-agnostic; STS is global but SDK requires a region—use us-east-1 safely.
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, err
	}
	return sts.NewFromConfig(cfg), nil
}

func resolveAccountID(profile string) (string, error) {
	cli, err := stsClient(profile)
	if err != nil {
		return "", err
	}
	out, err := cli.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Account), nil
}

func listAllFunctions(cli *lambda.Client) ([]lamtypes.FunctionConfiguration, error) {
	ctx := context.Background()
	var out []lamtypes.FunctionConfiguration
	p := lambda.NewListFunctionsPaginator(cli, &lambda.ListFunctionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Functions...)
	}
	return out, nil
}

func getRuntime(cli *lambda.Client, fn string) (string, error) {
	ctx := context.Background()
	cfg, err := cli.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
		FunctionName: aws.String(fn),
	})
	if err != nil {
		return "", err
	}
	return string(cfg.Runtime), nil
}

func updateAndWait(cli *lambda.Client, fn, target string, timeout, poll time.Duration) {
	ctx := context.Background()
	fmt.Printf("Updating %s to %s...\n", fn, target)
	_, err := cli.UpdateFunctionConfiguration(ctx, &lambda.UpdateFunctionConfigurationInput{
		FunctionName: aws.String(fn),
		Runtime:      lamtypes.Runtime(target),
	})
	if err != nil {
		fmt.Println("  update error:", err)
		return
	}
	deadline := time.Now().Add(timeout)
	for {
		cfg, err := cli.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
			FunctionName: aws.String(fn),
		})
		if err != nil {
			fmt.Println("  wait error:", err)
			return
		}
		switch cfg.LastUpdateStatus {
		case lamtypes.LastUpdateStatusSuccessful:
			fmt.Printf("%s updated successfully\n", fn)
			return
		case lamtypes.LastUpdateStatusFailed:
			fmt.Printf("%s update failed: %s\n", fn, aws.ToString(cfg.LastUpdateStatusReason))
			return
		}
		if time.Now().After(deadline) {
			fmt.Printf("Timed out waiting for %s\n", fn)
			return
		}
		time.Sleep(poll)
	}
}

// output: AccountID-first; profile optional
func printHeader(w *tabwriter.Writer, showProfile bool) {
	if showProfile {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", "AccountID", "Profile", "Region", "FunctionName", "CurrentRuntime")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", "---------", "-------", "------", "------------", "--------------")
	} else {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", "AccountID", "Region", "FunctionName", "CurrentRuntime")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", "---------", "------", "------------", "--------------")
	}
}

func printRow(w *tabwriter.Writer, accountID, profile, region, fn, rt string, showProfile bool) {
	if rt == "" {
		rt = "N/A"
	}
	if showProfile {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", accountID, profile, region, fn, rt)
	} else {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", accountID, region, fn, rt)
	}
}
