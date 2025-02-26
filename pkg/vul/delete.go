// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vul

import (
	"context"
	"fmt"

	ca "cloud.google.com/go/containeranalysis/apiv1"
	"github.com/GoogleCloudPlatform/aactl/pkg/types"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
	g "google.golang.org/genproto/googleapis/grafeas/v1"
)

// deleteNoteOccurrences deletes notes and occurrences. Used for debugging.
// nolint:unused
func deleteNoteOccurrences(ctx context.Context, opt *types.VulnerabilityOptions, list map[string]types.NoteOccurrences) error {
	c, err := ca.NewClient(ctx)
	if err != nil {
		return errors.Wrap(err, "error creating client")
	}
	defer c.Close()

	p := fmt.Sprintf("projects/%s", opt.Project)

	// Delete Notes
	for noteID := range list {
		noteName := fmt.Sprintf("%s/notes/%s", p, noteID)

		dr := &g.DeleteNoteRequest{
			Name: noteName,
		}
		_ = c.GetGrafeasClient().DeleteNote(ctx, dr)
	}

	// Delete Occurrences
	req := &g.ListOccurrencesRequest{
		Parent:   p,
		Filter:   fmt.Sprintf("resource_url=\"%s\"", opt.Source),
		PageSize: 1000,
	}
	it := c.GetGrafeasClient().ListOccurrences(ctx, req)
	for {
		resp, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}

		dr := &g.DeleteOccurrenceRequest{
			Name: resp.Name,
		}
		_ = c.GetGrafeasClient().DeleteOccurrence(ctx, dr)
	}

	return nil
}
