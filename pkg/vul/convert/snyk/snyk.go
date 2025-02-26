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

package snyk

import (
	"fmt"

	"github.com/GoogleCloudPlatform/aactl/pkg/types"
	"github.com/GoogleCloudPlatform/aactl/pkg/utils"
	"github.com/Jeffail/gabs/v2"
	"github.com/pkg/errors"
	g "google.golang.org/genproto/googleapis/grafeas/v1"
)

// Convert converts Snyk JSON to Grafeas Note/Occurrence format.
func Convert(s *utils.Source) (types.NoteOccurrencesMap, error) {
	if s == nil || s.Data == nil {
		return nil, errors.New("valid source required")
	}

	if !s.Data.Search("vulnerabilities").Exists() {
		return nil, errors.New("unable to find vulnerabilities in source data")
	}

	list := make(types.NoteOccurrencesMap, 0)

	for _, v := range s.Data.Search("vulnerabilities").Children() {
		cve := v.Search("identifiers", "CVE").Index(0).Data().(string)
		noteID := utils.GetPrefixNoteName(cve)

		// create note
		n := convertNote(s, v)

		// don't add notes with no CVSS score
		if n == nil || n.GetVulnerability().CvssScore == 0 {
			continue
		}

		// If cve is not found, add to map
		if _, ok := list[noteID]; !ok {
			list[noteID] = types.NoteOccurrences{Note: n}
		}
		nocc := list[noteID]
		occ := convertOccurrence(s, v, noteID)
		nocc.Occurrences = append(nocc.Occurrences, occ)
		list[noteID] = nocc
	}

	return list, nil
}

func convertNote(s *utils.Source, v *gabs.Container) *g.Note {
	cve := v.Search("identifiers", "CVE").Index(0).Data().(string)

	// Get cvss3 details from NVD
	var cvss3 *gabs.Container
	for _, detail := range v.Search("cvssDetails").Children() {
		if utils.ToString(detail.Search("assigner").Data()) == "NVD" {
			cvss3 = detail
		}
	}
	if cvss3 == nil {
		return nil
	}

	// create note
	n := g.Note{
		ShortDescription: cve,
		LongDescription:  utils.ToString(v.Search("CVSSv3").Data()),
		RelatedUrl: []*g.RelatedUrl{
			{
				Label: "Registry",
				Url:   s.URI,
			},
		},
		Type: &g.Note_Vulnerability{
			Vulnerability: &g.VulnerabilityNote{
				CvssVersion: g.CVSSVersion_CVSS_VERSION_3,
				CvssScore:   utils.ToFloat32(cvss3.Search("cvssV3BaseScore").Data()),
				// Details in Notes are not populated since we will never see the full list
				Details: []*g.VulnerabilityNote_Detail{
					{
						AffectedCpeUri:  "N/A",
						AffectedPackage: "N/A",
					},
				},
				Severity:         utils.ToGrafeasSeverity(v.Search("nvdSeverity").Data().(string)),
				SourceUpdateTime: utils.ToGRPCTime(cvss3.Search("modificationTime").Data()),
			},
		},
	} // end note

	// CVSSv3
	if cvss3.Search("cvssV3Vector").Data() != nil {
		n.GetVulnerability().CvssV3 = utils.ToCVSSv3(
			utils.ToFloat32(cvss3.Search("cvssV3BaseScore").Data()),
			cvss3.Search("cvssV3Vector").Data().(string),
		)
	}

	// References
	for _, r := range v.Search("references").Children() {
		n.RelatedUrl = append(n.RelatedUrl, &g.RelatedUrl{
			Url:   r.Search("url").Data().(string),
			Label: r.Search("title").Data().(string),
		})
	}

	return &n
}

func convertOccurrence(s *utils.Source, v *gabs.Container, noteID string) *g.Occurrence {
	cve := v.Search("identifiers", "CVE").Index(0).Data().(string)
	noteName := fmt.Sprintf("projects/%s/notes/%s", s.Project, noteID)

	// Get cvss3 details from NVD
	var cvss3 *gabs.Container
	for _, detail := range v.Search("cvssDetails").Children() {
		if utils.ToString(detail.Search("assigner").Data()) == "NVD" {
			cvss3 = detail
		}
	}
	if cvss3 == nil {
		return nil
	}

	// Create Occurrence
	o := g.Occurrence{
		ResourceUri: fmt.Sprintf("https://%s", s.URI),
		NoteName:    noteName,
		Details: &g.Occurrence_Vulnerability{
			Vulnerability: &g.VulnerabilityOccurrence{
				ShortDescription: cve,
				LongDescription:  utils.ToString(v.Search("CVSSv3").Data()),
				RelatedUrls: []*g.RelatedUrl{
					{
						Label: "Registry",
						Url:   s.URI,
					},
				},
				CvssVersion: g.CVSSVersion_CVSS_VERSION_3,
				CvssScore:   utils.ToFloat32(cvss3.Search("cvssV3BaseScore").Data()),
				// TODO: Set PackageType
				PackageIssue: []*g.VulnerabilityOccurrence_PackageIssue{{
					AffectedCpeUri:  makeCPE(v),
					AffectedPackage: v.Search("packageName").Data().(string),
					AffectedVersion: &g.Version{
						Name: v.Search("version").Data().(string),
						Kind: g.Version_NORMAL,
					},
					FixedCpeUri:  makeCPE(v),
					FixedPackage: v.Search("packageName").Data().(string),
					FixedVersion: &g.Version{
						Kind: g.Version_MAXIMUM,
					},
				}},
				Severity: utils.ToGrafeasSeverity(v.Search("nvdSeverity").Data().(string)),
				// TODO: What is the difference between severity and effective severity?
				EffectiveSeverity: utils.ToGrafeasSeverity(v.Search("nvdSeverity").Data().(string)),
			}},
	}

	// CVSSv3
	if cvss3.Search("cvssV3Vector").Data() != nil {
		o.GetVulnerability().Cvssv3 = utils.ToCVSS(
			utils.ToFloat32(cvss3.Search("cvssV3BaseScore").Data()),
			cvss3.Search("cvssV3Vector").Data().(string),
		)
	}

	// References
	for _, r := range v.Search("references").Children() {
		o.GetVulnerability().RelatedUrls = append(o.GetVulnerability().RelatedUrls, &g.RelatedUrl{
			Url:   r.Search("url").Data().(string),
			Label: r.Search("title").Data().(string),
		})
	}

	return &o
}

// makeCPE creates CPE from Snyk data as the OSS CLI does not generate CPEs
// NOTE: This is for demo purposes only and is not a complete CPE generator
// Ref: https://en.wikipedia.org/wiki/Common_Platform_Enumeration
// Schema: cpe:2.3:a:<vendor>:<product>:<version>:<update>:<edition>:<language>:<sw_edition>:<target_sw>:<target_hw>:<other>
func makeCPE(v *gabs.Container) string {
	pkgName := v.Search("name").Data().(string)
	pkgVersion := v.Search("version").Data().(string)

	return fmt.Sprintf("cpe:2.3:a:%s:%s:%s:*:*:*:*:*:*:*",
		pkgName,
		pkgName,
		pkgVersion)
}
