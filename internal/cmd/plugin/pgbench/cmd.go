/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package pgbench

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudnative-pg/cloudnative-pg/internal/cmd/plugin"
)

// NewCmd initializes the pgBench command
func NewCmd() *cobra.Command {
	run := &pgBenchRun{}

	pgBenchCmd := &cobra.Command{
		Use:     "pgbench CLUSTER [-- PGBENCH_COMMAND_ARGS...]",
		Short:   "Creates a pgbench job",
		Args:    validateCommandArgs,
		Long:    "Creates a pgbench job to run against the specified Postgres Cluster.",
		GroupID: plugin.GroupIDMiscellaneous,
		Example: jobExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			run.clusterName = args[0]
			run.pgBenchCommandArgs = args[1:]

			return run.execute(cmd.Context())
		},
	}

	pgBenchCmd.Flags().StringVar(
		&run.jobName,
		"job-name",
		"",
		"Name of the job, defaulting to: CLUSTER-pgbench-xxxx",
	)

	pgBenchCmd.Flags().StringVar(
		&run.jobName,
		"pgbench-job-name",
		"",
		"Name of the job, defaulting to: CLUSTER-pgbench-xxxx",
	)

	pgBenchCmd.Flags().StringVar(
		&run.dbName,
		"db-name",
		"app",
		"The name of the database that will be used by pgbench. Defaults to: app",
	)

	pgBenchCmd.Flags().Int32Var(
		&run.ttlSecondsAfterFinished,
		"ttl",
		0,
		"Time to live of the pgbench job. Defaults to no TTL.",
	)

	pgBenchCmd.Flags().BoolVar(
		&run.dryRun,
		"dry-run",
		false,
		"When true prints the job manifest instead of creating it",
	)

	pgBenchCmd.Flags().StringSliceVar(
		&run.nodeSelector,
		"node-selector",
		[]string{},
		"Node label selector in the <labelName>=<labelValue> format.",
	)
	_ = pgBenchCmd.Flags().MarkDeprecated("pgbench-job-name", "use job-name instead")

	return pgBenchCmd
}

func validateCommandArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		return err
	}

	if cmd.ArgsLenAtDash() > 1 {
		return fmt.Errorf("PGBENCH_COMMAND_ARGS should be passed after the -- delimiter")
	}

	return nil
}
