package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/domain"
	"wildframe/internal/evidence"
)

type selfcheckClient struct {
	base string
	http *http.Client
}

func runSelfcheck(ctx context.Context, base string) error {
	client := &selfcheckClient{base: base, http: &http.Client{Timeout: 3 * time.Second}}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	create := application.CreateCollectionCommand{CommandMeta: application.CommandMeta{Actor: "self-organizer", Role: application.RoleOrganizer, IdempotencyKey: "self-create"}, ReserveName: "自检保护区", CameraSite: "SELF-CAM-01", CapturedFrom: from, CapturedTo: from.Add(24 * time.Hour), RuleSetVersion: "selfcheck-v1", SeatA: "self-seat-a", SeatB: "self-seat-b"}
	var collection domain.ImageCollection
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections", create, &collection); err != nil {
		return fmt.Errorf("建档: %w", err)
	}
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
	var item domain.ImageEvidence
	if err := client.upload(ctx, collection, from.Add(time.Hour), payload, &item); err != nil {
		return fmt.Errorf("登记证据: %w", err)
	}
	collection.Version++
	a := application.SubmitAnnotationCommand{CommandMeta: application.CommandMeta{Actor: "self-seat-a", Role: application.RoleAnnotator, ExpectedVersion: collection.Version, IdempotencyKey: "self-ann-a"}, EvidenceID: item.EvidenceID, SeatID: "self-seat-a", SpeciesCode: "ursus-thibetanus", IndividualCount: 1, Confidence: .93, Identifiability: "identifiable"}
	var revision domain.AnnotationRevision
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/annotations", a, &revision); err != nil {
		return fmt.Errorf("席位 A 标注: %w", err)
	}
	collection.Version++
	b := application.SubmitAnnotationCommand{CommandMeta: application.CommandMeta{Actor: "self-seat-b", Role: application.RoleAnnotator, ExpectedVersion: collection.Version, IdempotencyKey: "self-ann-b"}, EvidenceID: item.EvidenceID, SeatID: "self-seat-b", SpeciesCode: "vulpes-vulpes", IndividualCount: 1, Confidence: .88, Identifiability: "identifiable"}
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/annotations", b, &revision); err != nil {
		return fmt.Errorf("席位 B 标注: %w", err)
	}
	collection.Version++
	decisionCommand := application.AdjudicateCommand{CommandMeta: application.CommandMeta{Actor: "self-expert", Role: application.RoleExpert, ExpectedVersion: collection.Version, IdempotencyKey: "self-decision"}, EvidenceID: item.EvidenceID, Action: "new_conclusion", FinalSpeciesCode: "ursus-thibetanus", FinalCount: 1, Rationale: "自检专家依据体色和胸斑完成裁决"}
	var decision domain.AdjudicationDecision
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/adjudications", decisionCommand, &decision); err != nil {
		return fmt.Errorf("专家仲裁: %w", err)
	}
	collection.Version++
	review := application.ReviewCommand{CommandMeta: application.CommandMeta{Actor: "self-expert", Role: application.RoleExpert, ExpectedVersion: collection.Version, IdempotencyKey: "self-review"}}
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/review", review, &collection); err != nil {
		return fmt.Errorf("专家复核: %w", err)
	}
	var preview application.ManifestPreview
	if err := client.json(ctx, http.MethodGet, "/api/v1/collections/"+collection.CollectionID+"/manifest", nil, &preview); err != nil {
		return fmt.Errorf("清单预览: %w", err)
	}
	freeze := application.FreezeCommand{CommandMeta: application.CommandMeta{Actor: "self-publisher", Role: application.RolePublisher, ExpectedVersion: collection.Version, IdempotencyKey: "self-freeze"}, ManifestDigest: preview.Digest}
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/freeze", freeze, &collection); err != nil {
		return fmt.Errorf("冻结清单: %w", err)
	}
	issue := application.IssueCommand{CommandMeta: application.CommandMeta{Actor: "self-publisher", Role: application.RolePublisher, ExpectedVersion: collection.Version, IdempotencyKey: "self-issue"}}
	var credential evidence.CredentialEnvelope
	if err := client.json(ctx, http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/credential", issue, &credential); err != nil {
		return fmt.Errorf("签发凭据: %w", err)
	}
	var verification application.VerificationResult
	if err := client.json(ctx, http.MethodPost, "/api/v1/credentials/verify", credential, &verification); err != nil {
		return fmt.Errorf("验证凭据: %w", err)
	}
	var view application.CollectionView
	if err := client.json(ctx, http.MethodGet, "/api/v1/collections/"+collection.CollectionID, nil, &view); err != nil {
		return fmt.Errorf("读取终态: %w", err)
	}
	if !verification.Valid || view.Collection.Status != domain.StatusReleased || len(view.Audit) < 8 {
		return fmt.Errorf("终态断言失败：status=%s valid=%v audit=%d", view.Collection.Status, verification.Valid, len(view.Audit))
	}
	return nil
}

func (c *selfcheckClient) json(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(response.Body)
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, raw)
	}
	if output != nil {
		return json.NewDecoder(response.Body).Decode(output)
	}
	return nil
}

func (c *selfcheckClient) upload(ctx context.Context, collection domain.ImageCollection, captured time.Time, payload []byte, output any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "selfcheck.jpg")
	if err != nil {
		return err
	}
	if _, err := part.Write(payload); err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	fields := map[string]string{"capturedAt": captured.Format(time.RFC3339), "cameraSite": collection.CameraSite, "sha256Digest": hex.EncodeToString(sum[:]), "pixelWidth": "640", "pixelHeight": "480", "actor": "self-organizer", "role": "organizer", "expectedVersion": strconv.FormatInt(collection.Version, 10), "idempotencyKey": "self-upload"}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/collections/"+collection.CollectionID+"/evidence", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(response.Body)
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, raw)
	}
	return json.NewDecoder(response.Body).Decode(output)
}
