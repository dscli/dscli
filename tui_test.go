package main

import (
	"testing"

	"gitcode.com/dscli/dscli/internal/context"
	"github.com/spf13/cobra"
)

// ─── tuiPreRunE 测试 ────────────────────────────────────────────────────────

func TestTUIPreRunEValidChatModel(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("model", context.ModelDeepseekChat, "")
	cmd.Flags().Bool("verbose", false, "")
	ctx := t.Context()
	cmd.SetContext(ctx)

	// 使用默认值（deepseek-chat）
	err := tuiPreRunE(cmd, nil)
	if err != nil {
		t.Fatalf("tuiPreRunE with default model should succeed, got: %v", err)
	}

	newCtx := cmd.Context()
	modelID := context.ContextValue(newCtx, context.CurrentModelIDKey, int64(-1))
	if modelID != DeepseekChat {
		t.Errorf("expected modelID=%d (DeepseekChat), got %d", DeepseekChat, modelID)
	}
	modelName := context.ContextValue(newCtx, context.CurrentModelNameKey, "")
	if modelName != context.ModelDeepseekChat {
		t.Errorf("expected modelName=%q, got %q", context.ModelDeepseekChat, modelName)
	}
}

func TestTUIPreRunEValidReasonerModel(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("model", context.ModelDeepseekChat, "")
	cmd.Flags().Bool("verbose", false, "")
	ctx := t.Context()
	cmd.SetContext(ctx)

	if err := cmd.Flags().Set("model", context.ModelDeepseekReasoner); err != nil {
		t.Fatal(err)
	}

	err := tuiPreRunE(cmd, nil)
	if err != nil {
		t.Fatalf("tuiPreRunE with reasoner model should succeed, got: %v", err)
	}

	newCtx := cmd.Context()
	modelID := context.ContextValue(newCtx, context.CurrentModelIDKey, int64(-1))
	if modelID != DeepseekReasoner {
		t.Errorf("expected modelID=%d (DeepseekReasoner), got %d", DeepseekReasoner, modelID)
	}
	modelName := context.ContextValue(newCtx, context.CurrentModelNameKey, "")
	if modelName != context.ModelDeepseekReasoner {
		t.Errorf("expected modelName=%q, got %q", context.ModelDeepseekReasoner, modelName)
	}
}

func TestTUIPreRunEInvalidModel(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("model", context.ModelDeepseekChat, "")
	cmd.Flags().Bool("verbose", false, "")
	ctx := t.Context()
	cmd.SetContext(ctx)

	if err := cmd.Flags().Set("model", "gpt-4"); err != nil {
		t.Fatal(err)
	}

	err := tuiPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("tuiPreRunE with invalid model should return error")
	}
}

func TestTUIPreRunEInvalidModelVerbose(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("model", context.ModelDeepseekChat, "")
	cmd.Flags().Bool("verbose", true, "")
	ctx := t.Context()
	cmd.SetContext(ctx)

	if err := cmd.Flags().Set("model", "unknown-model"); err != nil {
		t.Fatal(err)
	}

	err := tuiPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("tuiPreRunE with invalid model should return error even in verbose mode")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// ─── tuiRunE 测试 ───────────────────────────────────────────────────────────

func TestTUIRunENilClient(t *testing.T) {
	origClient := DeepseekClient
	defer func() { DeepseekClient = origClient }()

	DeepseekClient = nil

	cmd := &cobra.Command{}
	cmd.Flags().Int("histsize", 8, "")
	ctx := t.Context()
	ctx = context.WithValue(ctx, context.HistSizeKey, 8)
	cmd.SetContext(ctx)

	err := tuiRunE(cmd, nil)
	if err == nil {
		t.Fatal("tuiRunE with nil client should return error")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// ─── tui 命令注册测试 ──────────────────────────────────────────────────────

func TestTUICommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "tui" {
			found = true

			modelFlag := cmd.Flags().Lookup("model")
			if modelFlag == nil {
				t.Error("tui command missing --model flag")
			}

			histFlag := cmd.Flags().Lookup("histsize")
			if histFlag == nil {
				t.Error("tui command missing --histsize flag")
			}

			if cmd.Short == "" {
				t.Error("tui command Short should not be empty")
			}
			if cmd.Long == "" {
				t.Error("tui command Long should not be empty")
			}

			break
		}
	}
	if !found {
		t.Error("tui command not registered in rootCmd")
	}
}

func TestTUICommandHandlersSet(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "tui" {
			if cmd.PreRunE == nil {
				t.Error("tui command PreRunE should not be nil")
			}
			if cmd.RunE == nil {
				t.Error("tui command RunE should not be nil")
			}
			return
		}
	}
	t.Error("tui command not found")
}

// ─── tuiPreRunE 边界测试 ──────────────────────────────────────────────────

func TestTUIPreRunEFlagError(t *testing.T) {
	cmd := &cobra.Command{} // 没有注册任何 flags
	ctx := t.Context()
	cmd.SetContext(ctx)

	err := tuiPreRunE(cmd, nil)
	if err == nil {
		t.Fatal("tuiPreRunE without model flag should return error")
	}
}

func TestTUIPreRunEModelIDSetCorrectly(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		wantModelID   int64
		wantModelName string
		wantErr       bool
	}{
		{
			name:          "chat模型",
			model:         context.ModelDeepseekChat,
			wantModelID:   DeepseekChat,
			wantModelName: context.ModelDeepseekChat,
			wantErr:       false,
		},
		{
			name:          "reasoner模型",
			model:         context.ModelDeepseekReasoner,
			wantModelID:   DeepseekReasoner,
			wantModelName: context.ModelDeepseekReasoner,
			wantErr:       false,
		},
		{
			name:    "无效模型",
			model:   "invalid-model-name",
			wantErr: true,
		},
		{
			name:    "空模型名",
			model:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("model", context.ModelDeepseekChat, "")
			cmd.Flags().Bool("verbose", false, "")
			ctx := t.Context()
			cmd.SetContext(ctx)

			if err := cmd.Flags().Set("model", tt.model); err != nil {
				t.Fatal(err)
			}

			err := tuiPreRunE(cmd, nil)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			newCtx := cmd.Context()
			gotModelID := context.ContextValue(newCtx, context.CurrentModelIDKey, int64(-999))
			if gotModelID != tt.wantModelID {
				t.Errorf("modelID = %d, want %d", gotModelID, tt.wantModelID)
			}
			gotModelName := context.ContextValue(newCtx, context.CurrentModelNameKey, "")
			if gotModelName != tt.wantModelName {
				t.Errorf("modelName = %q, want %q", gotModelName, tt.wantModelName)
			}
		})
	}
}

// ─── tui 命令 flags 默认值测试 ─────────────────────────────────────────────

func TestTUICommandDefaultFlags(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() != "tui" {
			continue
		}

		modelFlag := cmd.Flags().Lookup("model")
		if modelFlag == nil {
			t.Fatal("missing --model flag")
		}
		if modelFlag.DefValue != context.ModelDeepseekChat {
			t.Errorf("--model default = %q, want %q", modelFlag.DefValue, context.ModelDeepseekChat)
		}

		histFlag := cmd.Flags().Lookup("histsize")
		if histFlag == nil {
			t.Fatal("missing --histsize flag")
		}
		if histFlag.DefValue != "8" {
			t.Errorf("--histsize default = %q, want \"8\"", histFlag.DefValue)
		}
		return
	}
	t.Error("tui command not found")
}
