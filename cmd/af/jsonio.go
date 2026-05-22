package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

type jsonEnvelope struct {
	Data   any `json:"data"`
	Schema int `json:"schema"`
}

func writeJSONEnvelope(cmd *cobra.Command, schema int, data any) error {
	payload := jsonEnvelope{Schema: schema, Data: data}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("json envelope: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
	if err != nil {
		return fmt.Errorf("json envelope write: %w", err)
	}
	return nil
}
