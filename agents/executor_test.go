package agents_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/tools"
	"github.com/tmc/langchaingo/tools/serpapi"
)

type testAgent struct {
	actions    []schema.AgentAction
	finish     *schema.AgentFinish
	err        error
	inputKeys  []string
	outputKeys []string

	recordedIntermediateSteps []schema.AgentStep
	recordedInputs            map[string]string
	numPlanCalls              int
}

func (a *testAgent) Plan(
	_ context.Context,
	intermediateSteps []schema.AgentStep,
	inputs map[string]string,
) ([]schema.AgentAction, *schema.AgentFinish, error) {
	a.recordedIntermediateSteps = intermediateSteps
	a.recordedInputs = inputs
	a.numPlanCalls++

	return a.actions, a.finish, a.err
}

func (a testAgent) GetInputKeys() []string {
	return a.inputKeys
}

func (a testAgent) GetOutputKeys() []string {
	return a.outputKeys
}

func (a *testAgent) GetTools() []tools.Tool {
	return nil
}

func TestExecutorWithErrorHandler(t *testing.T) {
	t.Parallel()

	a := &testAgent{
		err: agents.ErrUnableToParseOutput,
	}
	executor := agents.NewExecutor(
		a,
		agents.WithMaxIterations(3),
		agents.WithParserErrorHandler(agents.NewParserErrorHandler(nil)),
	)

	_, err := chains.Call(context.Background(), executor, nil)
	require.ErrorIs(t, err, agents.ErrNotFinished)
	require.Equal(t, 3, a.numPlanCalls)
	require.Equal(t, []schema.AgentStep{
		{Observation: agents.ErrUnableToParseOutput.Error()},
		{Observation: agents.ErrUnableToParseOutput.Error()},
	}, a.recordedIntermediateSteps)
}

func TestExecutorWithMRKLAgent(t *testing.T) {
	t.Parallel()

	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	if serpapiKey := os.Getenv("SERPAPI_API_KEY"); serpapiKey == "" {
		t.Skip("SERPAPI_API_KEY not set")
	}

	llm, err := openai.New()
	require.NoError(t, err)

	searchTool, err := serpapi.New()
	require.NoError(t, err)

	calculator := tools.Calculator{}

	a, err := agents.Initialize(
		llm,
		[]tools.Tool{searchTool, calculator},
		agents.ZeroShotReactDescription,
	)
	require.NoError(t, err)

	result, err := chains.Run(context.Background(), a, "What is 5 plus 3? Please calculate this.") //nolint:lll
	require.NoError(t, err)

	t.Logf("MRKL Agent response: %s", result)
	// Simple calculation: 5 + 3 = 8
	require.True(t, strings.Contains(result, "8"), "expected calculation result 8 in response")
}

func TestExecutorWithOpenAIFunctionAgent(t *testing.T) {
	t.Parallel()

	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	if serpapiKey := os.Getenv("SERPAPI_API_KEY"); serpapiKey == "" {
		t.Skip("SERPAPI_API_KEY not set")
	}

	llm, err := openai.New()
	require.NoError(t, err)

	searchTool, err := serpapi.New()
	require.NoError(t, err)

	calculator := tools.Calculator{}

	toolList := []tools.Tool{searchTool, calculator}

	a := agents.NewOpenAIFunctionsAgent(llm,
		toolList,
		agents.NewOpenAIOption().WithSystemMessage("you are a helpful assistant"),
		agents.NewOpenAIOption().WithExtraMessages([]prompts.MessageFormatter{
			prompts.NewHumanMessagePromptTemplate("please be strict", nil),
		}),
	)

	e := agents.NewExecutor(a)
	require.NoError(t, err)

	result, err := chains.Run(context.Background(), e, "when was the Go programming language tagged version 1.0?") //nolint:lll
	require.NoError(t, err)

	t.Logf("Result: %s", result)

	require.True(t, strings.Contains(result, "2012") || strings.Contains(result, "March"),
		"correct answer 2012 or March not in response")
}
