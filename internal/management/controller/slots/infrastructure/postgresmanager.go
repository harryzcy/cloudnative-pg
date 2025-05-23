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

package infrastructure

import (
	"context"
	"database/sql"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	v1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

// List the available replication slots
func List(ctx context.Context, db *sql.DB, config *v1.ReplicationSlotsConfiguration) (ReplicationSlotList, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT slot_name, slot_type, active, coalesce(restart_lsn::TEXT, '') AS restart_lsn,
            xmin IS NOT NULL OR catalog_xmin IS NOT NULL AS holds_xmin
            FROM pg_catalog.pg_replication_slots
            WHERE NOT temporary AND slot_type = 'physical'`,
	)
	if err != nil {
		return ReplicationSlotList{}, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var status ReplicationSlotList
	for rows.Next() {
		var slot ReplicationSlot
		err := rows.Scan(
			&slot.SlotName,
			&slot.Type,
			&slot.Active,
			&slot.RestartLSN,
			&slot.HoldsXmin,
		)
		if err != nil {
			return ReplicationSlotList{}, err
		}

		slot.IsHA = strings.HasPrefix(slot.SlotName, config.HighAvailability.GetSlotPrefix())
		isFilteredByUser, err := config.SynchronizeReplicas.IsExcludedByUser(slot.SlotName)
		if err != nil {
			return status, err
		}
		if !slot.IsHA && isFilteredByUser {
			continue
		}

		status.Items = append(status.Items, slot)
	}

	if rows.Err() != nil {
		return ReplicationSlotList{}, rows.Err()
	}

	return status, nil
}

// Update the replication slot
func Update(ctx context.Context, db *sql.DB, slot ReplicationSlot) error {
	contextLog := log.FromContext(ctx).WithName("updateSlot")
	contextLog.Trace("Invoked", "slot", slot)
	if slot.RestartLSN == "" {
		return nil
	}

	_, err := db.ExecContext(ctx, "SELECT pg_catalog.pg_replication_slot_advance($1, $2)", slot.SlotName, slot.RestartLSN)
	return err
}

// Create the replication slot
func Create(ctx context.Context, db *sql.DB, slot ReplicationSlot) error {
	contextLog := log.FromContext(ctx).WithName("createSlot")
	contextLog.Trace("Invoked", "slot", slot)

	_, err := db.ExecContext(ctx, "SELECT pg_catalog.pg_create_physical_replication_slot($1, $2)",
		slot.SlotName, slot.RestartLSN != "")
	return err
}

// Delete the replication slot
func Delete(ctx context.Context, db *sql.DB, slot ReplicationSlot) error {
	contextLog := log.FromContext(ctx).WithName("dropSlot")
	contextLog.Trace("Invoked", "slot", slot)
	if slot.Active {
		return nil
	}

	_, err := db.ExecContext(ctx, "SELECT pg_catalog.pg_drop_replication_slot($1)", slot.SlotName)
	return err
}
