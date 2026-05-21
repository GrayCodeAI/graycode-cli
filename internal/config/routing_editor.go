package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/router"
)

// LoadRoutingPolicyJSON returns the routing section of provider.json as indented JSON.
func LoadRoutingPolicyJSON() (string, error) {
	cfg := eyriecfg.LoadProviderConfig("")
	cfg = eyriecfg.EnsureDeploymentConfigV2(cfg)
	if cfg == nil {
		return defaultRoutingPolicyJSON(), nil
	}
	if cfg.Routing == nil {
		return defaultRoutingPolicyJSON(), nil
	}
	data, err := json.MarshalIndent(cfg.Routing, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func defaultRoutingPolicyJSON() string {
	cfg := &eyriecfg.ProviderConfig{}
	cfg = eyriecfg.EnsureDeploymentConfigV2(cfg)
	if cfg != nil && cfg.Routing != nil {
		data, _ := json.MarshalIndent(cfg.Routing, "", "  ")
		return string(data)
	}
	tmpl := &eyriecfg.RoutingPolicy{
		Providers: map[string][]eyriecfg.RoutingStage{
			"anthropic": {{
				Deployments: []eyriecfg.DeploymentChoice{
					{DeploymentID: "anthropic-direct", Weight: 100},
				},
				Retries: 1,
			}},
		},
	}
	data, _ := json.MarshalIndent(tmpl, "", "  ")
	return string(data)
}

// SaveRoutingPolicyJSON validates and persists routing into provider.json.
func SaveRoutingPolicyJSON(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("routing JSON is empty")
	}
	var policy eyriecfg.RoutingPolicy
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&policy); err != nil {
		return fmt.Errorf("invalid routing JSON: %w", err)
	}
	if err := validateRoutingPolicy(&policy); err != nil {
		return err
	}

	path := eyriecfg.GetProviderConfigPath()
	cfg, err := eyriecfg.LoadProviderConfigWithError(path)
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &eyriecfg.ProviderConfig{}
	}
	cfg = eyriecfg.EnsureDeploymentConfigV2(cfg)
	cfg.Routing = &policy
	cfg.ConfigVersion = 2
	return eyriecfg.SaveProviderConfig(cfg, path)
}

func validateRoutingPolicy(policy *eyriecfg.RoutingPolicy) error {
	if policy == nil {
		return fmt.Errorf("routing policy is nil")
	}
	compiled, err := loadEyrieCatalogV1(context.Background(), false)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	checkStages := func(stages []router.RoutingStage, scope string) error {
		for i, stage := range stages {
			if len(stage.Deployments) == 0 {
				return fmt.Errorf("%s stage %d has no deployments", scope, i)
			}
			for _, choice := range stage.Deployments {
				if choice.DeploymentID == "" {
					return fmt.Errorf("%s stage %d has empty deployment_id", scope, i)
				}
				if choice.Weight <= 0 {
					return fmt.Errorf("%s stage %d: deployment %q weight must be > 0", scope, i, choice.DeploymentID)
				}
				if compiled.DeploymentsByID[choice.DeploymentID].ID == "" {
					return fmt.Errorf("%s stage %d: unknown deployment %q", scope, i, choice.DeploymentID)
				}
			}
		}
		return nil
	}
	for modelID, stages := range policy.Models {
		if len(stages) == 0 {
			continue
		}
		if err := checkStages(convertStages(stages), "models["+modelID+"]"); err != nil {
			return err
		}
	}
	for providerID, stages := range policy.Providers {
		if len(stages) == 0 {
			continue
		}
		if err := checkStages(convertStages(stages), "providers["+providerID+"]"); err != nil {
			return err
		}
	}
	if len(policy.Default) > 0 {
		if err := checkStages(convertStages(policy.Default), "default"); err != nil {
			return err
		}
	}
	return nil
}

func convertStages(stages []eyriecfg.RoutingStage) []router.RoutingStage {
	out := make([]router.RoutingStage, len(stages))
	for i, stage := range stages {
		out[i].Retries = stage.Retries
		out[i].Deployments = make([]router.DeploymentChoice, len(stage.Deployments))
		for j, d := range stage.Deployments {
			out[i].Deployments[j] = router.DeploymentChoice{DeploymentID: d.DeploymentID, Weight: d.Weight}
		}
	}
	return out
}
