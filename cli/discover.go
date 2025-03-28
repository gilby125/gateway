package cli

import (
	"context"
	_ "embed"
	"github.com/centralmind/gateway/prompter"
	"github.com/centralmind/gateway/providers"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/centralmind/gateway/logger"
	"golang.org/x/xerrors"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	gw_model "github.com/centralmind/gateway/model"
)

var (
	// CLI-colors
	red    string = "\033[31m"
	green         = "\033[32m"
	cyan          = "\033[36m"
	yellow        = "\033[33m"
	violet        = "\033[35m"
	reset         = "\033[0m" // reset color
)

func init() {
	// Configure logrus for nicer output
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		ForceColors:            true,
		FullTimestamp:          false,
		TimestampFormat:        "",
		DisableTimestamp:       true,
	})
}

func Discover() *cobra.Command {
	var configPath string
	var tables string
	var aiProvider string
	var aiEndpoint string
	var aiAPIKey string
	var aiModel string
	var aiMaxTokens int
	var aiTemperature float32
	var aiReasoning bool
	var bedrockRegion string
	var vertexAIRegion string
	var vertexAIProject string
	var output string
	var extraPrompt string
	var promptFile string
	var llmLogFile string

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover generates gateway config",
		Args:  cobra.MatchAll(cobra.ExactArgs(0)),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			startTime := time.Now()

			logrus.Info("\r\n")
			logrus.Info("🚀 Verify Discovery Process")

			configRaw, err := os.ReadFile(configPath)
			if err != nil {
				return err
			}

			resolvedTables, connector, err := TablesData(splitTables(tables), configRaw)
			if err != nil {
				return xerrors.Errorf("unable to verify connection: %w", err)
			}

			databaseType := inferType(configRaw)

			logrus.Info("Step 4: Prepare the prompt for the AI")
			discoverPrompt := prompter.DiscoverPrompt(connector, extraPrompt, resolvedTables, prompter.SchemaFromConfig(connector.Config()))
			if err := saveToFile(promptFile, discoverPrompt); err != nil {
				logrus.Error("failed to save prompt:", err)
			}

			logrus.Debugf("Prompt saved locally to %s", promptFile)
			logrus.Info("✅ Step 4 completed. Done.")
			logrus.Info("\r\n")

			// Call API
			logrus.Info("Step 5: Use AI to design the API")
			response, err := makeDiscoverQuery(DiscoverQueryParams{
				LLMLogFile:    llmLogFile,
				Provider:      aiProvider,
				Endpoint:      aiEndpoint,
				APIKey:        aiAPIKey,
				Model:         aiModel,
				MaxTokens:     aiMaxTokens,
				Temperature:   aiTemperature,
				Reasoning:     aiReasoning,
				BedrockRegion: bedrockRegion,
				VertexRegion:  vertexAIRegion,
				VertexProject: vertexAIProject,
			}, discoverPrompt)

			if err != nil {
				logrus.Error("Failed to call the LLM:", err)
				return err
			}

			config := response.Config

			// Show generated API endpoints
			var apiEndpoints int
			logrus.Info("API Functions Created:")
			for _, table := range config.Database.Tables {
				for _, endpoint := range table.Endpoints {
					logrus.Infof("  - "+cyan+"%s"+reset+" "+violet+"%s"+reset+" - %s", endpoint.HTTPMethod, endpoint.HTTPPath, endpoint.Summary)
					apiEndpoints++
				}
			}

			config.Database.Type = databaseType
			config.Database.Connection = string(configRaw)

			// Save configuration
			configData, err := yaml.Marshal(config)
			if err != nil {
				logrus.Error("yaml failed:", err)
				return err
			}

			if err := saveToFile(output, string(configData)); err != nil {
				logrus.Error("failed:", err)
				return err
			}
			logrus.Info("\r\n")
			logrus.Infof("API schema saved to: "+cyan+"%s"+reset, output)

			logrus.Info("✅ Step 5: API Specification Generation Completed!")
			logrus.Info("\r\n")
			// Show statistics
			duration := time.Since(startTime)

			logrus.Info("✅ All steps completed. Done.")
			logrus.Info("\r\n")
			logrus.Info("--- Execution Statistics ---")
			logrus.Infof("Total time taken: "+yellow+"%v"+reset, duration.Round(time.Second))
			logrus.Infof(
				"Tokens used: "+yellow+"%d"+reset+" (Estimated cost: "+violet+"$%.4f"+reset+")",
				response.Conversation.Usage.TotalTokens,
				response.CostEstimate,
			)
			logrus.Infof("Tables processed: "+yellow+"%d"+reset, len(resolvedTables))
			logrus.Infof("API methods created: "+yellow+"%d"+reset, apiEndpoints)

			// Count PII columns from the generated config
			var piiColumnsCount int
			for _, table := range config.Database.Tables {
				for _, column := range table.Columns {
					if column.PII {
						piiColumnsCount++
					}
				}
			}

			logrus.Infof("Total number of columns containing PII data: "+yellow+"%d"+reset, piiColumnsCount)

			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "connection.yaml", "Path to connection yaml file")
	cmd.Flags().StringVar(&tables, "tables", "", "Comma-separated list of tables to include (e.g. 'table1,table2,table3')")

	/*
		AI provider options:
	*/
	cmd.Flags().StringVar(&aiProvider, "ai-provider", "openai", "AI provider to use")
	cmd.Flags().StringVar(&aiEndpoint, "ai-endpoint", "", "Custom OpenAI-compatible API endpoint URL")
	cmd.Flags().StringVar(&aiAPIKey, "ai-api-key", "", "AI API token")
	cmd.Flags().StringVar(&bedrockRegion, "bedrock-region", "", "Bedrock region")
	cmd.Flags().StringVar(&vertexAIRegion, "vertexai-region", "", "Vertex AI region")
	cmd.Flags().StringVar(&vertexAIProject, "vertexai-project", "", "Vertex AI project")
	cmd.Flags().StringVar(&aiModel, "ai-model", "", "AI model to use")
	cmd.Flags().IntVar(&aiMaxTokens, "ai-max-tokens", 0, "Maximum tokens to use")
	cmd.Flags().Float32Var(&aiTemperature, "ai-temperature", -1.0, "AI temperature")
	cmd.Flags().BoolVar(&aiReasoning, "ai-reasoning", true, "Enable reasoning")

	cmd.Flags().StringVar(&output, "output", "gateway.yaml", "Resulted YAML path")
	cmd.Flags().StringVar(&extraPrompt, "prompt", "generate reasonable set of APIs for this data", "Custom input to generate APIs")
	cmd.Flags().StringVar(&promptFile, "prompt-file", filepath.Join(logger.DefaultLogDir(), "prompt_default.txt"), "Path to save the generated prompt")
	cmd.Flags().StringVar(&llmLogFile, "llm-log", filepath.Join(logger.DefaultLogDir(), "llm_raw_response.log"), "Path to save the raw LLM response")

	return cmd
}

type DiscoverQueryParams struct {
	LLMLogFile    string
	Provider      string
	Endpoint      string
	APIKey        string
	Model         string
	MaxTokens     int
	Temperature   float32
	Reasoning     bool
	BedrockRegion string
	VertexRegion  string
	VertexProject string
}

type DiscoverQueryResponse struct {
	Config       *gw_model.Config
	Conversation *providers.ConversationResponse
	RawContent   string
	CostEstimate float64
}

func makeDiscoverQuery(params DiscoverQueryParams, prompt string) (DiscoverQueryResponse, error) {
	provider, err := providers.NewModelProvider(providers.ModelProviderConfig{
		Name:            params.Provider,
		APIKey:          params.APIKey,
		Endpoint:        params.Endpoint,
		BedrockRegion:   params.BedrockRegion,
		VertexAIRegion:  params.VertexRegion,
		VertexAIProject: params.VertexProject,
	})

	if err != nil {
		logrus.Fatalf("Failed to initialize provider: %v", err)
	}

	logrus.Infof("Calling provider: %s", provider.GetName())

	done := make(chan bool)
	go startSpinner("Thinking. The process can take a few minutes to finish", done)

	request := &providers.ConversationRequest{
		ModelId:      params.Model,
		Reasoning:    params.Reasoning,
		MaxTokens:    params.MaxTokens,
		Temperature:  params.Temperature,
		JsonResponse: true,
		System:       "You must always respond in pure JSON. No markdown, no comments, no explanations.",
		Messages: []providers.Message{
			{
				Role: providers.UserRole,
				Content: []providers.ContentBlock{
					&providers.ContentBlockText{
						Value: prompt,
					},
				},
			},
		},
	}

	llmResponse, err := provider.Chat(context.Background(), request)
	if err != nil {
		log.Fatalf("Failed to call LLM: %v", err)
	}

	done <- true

	var responseContentBuilder strings.Builder
	for _, contentBlock := range llmResponse.Content {
		if textBlock, ok := contentBlock.(*providers.ContentBlockText); ok {
			responseContentBuilder.WriteString(textBlock.Value)
		}
	}

	rawContent := strings.TrimSpace(responseContentBuilder.String())

	if err := os.WriteFile(params.LLMLogFile, []byte(rawContent), 0644); err != nil {
		logrus.Error("Failed to save LLM response:", err)
	}

	costEstimate := provider.CostEstimate(llmResponse.ModelId, *llmResponse.Usage)

	logrus.WithFields(logrus.Fields{
		"Total tokens":  llmResponse.Usage.TotalTokens,
		"Input tokens":  llmResponse.Usage.InputTokens,
		"Output tokens": llmResponse.Usage.OutputTokens,
	}).Info("LLM usage:")

	var response gw_model.Config
	if err := yaml.Unmarshal([]byte(rawContent), &response); err != nil {
		return DiscoverQueryResponse{
			Config:       nil,
			Conversation: llmResponse,
			RawContent:   rawContent,
			CostEstimate: costEstimate,
		}, xerrors.Errorf("unable to unmarshal response: %w", err)
	}

	return DiscoverQueryResponse{
		Config:       &response,
		Conversation: llmResponse,
		RawContent:   rawContent,
		CostEstimate: costEstimate,
	}, nil

}
