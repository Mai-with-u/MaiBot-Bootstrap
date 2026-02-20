package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"maibot/internal/config"
	"maibot/internal/logging"
	"maibot/internal/modules"
	"maibot/internal/version"
)

const (
	instanceProc  = "run-single"
	defaultName   = "main"
	configVersion = 1
)

type App struct {
	cfg         config.Config
	log         *logging.Logger
	instanceLog *logging.Logger
	updateLog   *logging.Logger
	cleanupLog  *logging.Logger
	modulesLog  *logging.Logger
}

func New() (*App, error) {
	cfg, err := config.LoadOrCreate()
	if err != nil {
		return nil, err
	}
	rootLog, err := logging.NewRoot(logging.Options{
		FilePath:       cfg.Logging.FilePath,
		MaxSizeMB:      cfg.Logging.MaxSizeMB,
		RetentionDays:  cfg.Logging.RetentionDays,
		MaxBackupFiles: cfg.Logging.MaxBackupFiles,
	})
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:         cfg,
		log:         rootLog.Module("app"),
		instanceLog: rootLog.Module("instance"),
		updateLog:   rootLog.Module("update"),
		cleanupLog:  rootLog.Module("cleanup"),
		modulesLog:  rootLog.Module("modules"),
	}, nil
}

func (a *App) Run(args []string) {
	if err := a.Execute(args); err != nil {
		a.log.Fatalf("command failed: %v", err)
	}
}

func (a *App) Execute(args []string) error {
	if err := a.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	cmd := a.newRootCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}

func (a *App) newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "maibot",
		Short: "MaiBot CLI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.runInteractiveTUI()
			return nil
		},
	}

	var chdirPath string
	root.PersistentFlags().StringVarP(&chdirPath, "directory", "C", "", "Run as if maibot was started in this path")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(chdirPath) == "" {
			return nil
		}
		abs, err := filepath.Abs(chdirPath)
		if err != nil {
			return err
		}
		if st, err := os.Stat(abs); err != nil {
			return err
		} else if !st.IsDir() {
			return fmt.Errorf("-C path is not a directory: %s", abs)
		}
		return os.Chdir(abs)
	}

	root.AddCommand(&cobra.Command{Use: "init", Aliases: []string{"install", "create"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.installInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("single workspace initialized")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "start", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.startInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("workspace started")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "stop", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.stopInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("workspace stopped")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "restart", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.restartInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("workspace restarted")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.statusInstance(defaultName)
	}})

	logs := &cobra.Command{Use: "logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		tail, _ := cmd.Flags().GetInt("tail")
		return a.logsInstance(defaultName, tail)
	}}
	logs.Flags().Int("tail", 50, "Tail lines")
	root.AddCommand(logs)

	root.AddCommand(&cobra.Command{Use: "update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.updateInstance(defaultName); err != nil {
			return err
		}
		a.updateLog.Okf("workspace updated")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "upgrade", Aliases: []string{"self-update"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.selfUpdate(); err != nil {
			return err
		}
		a.updateLog.Okf("maibot updated successfully")
		return nil
	}})

	serviceCmd := &cobra.Command{Use: "service", Short: "Manage OS service for single workspace"}
	serviceCmd.AddCommand(&cobra.Command{Use: "install", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("install", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "uninstall", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("uninstall", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "start", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("start", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "stop", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("stop", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("status", defaultName)
	}})
	root.AddCommand(serviceCmd)

	cleanup := &cobra.Command{Use: "cleanup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		testArtifacts, _ := cmd.Flags().GetBool("test-artifacts")
		if !testArtifacts {
			return fmt.Errorf("usage: maibot cleanup --test-artifacts")
		}
		if err := a.cleanup(); err != nil {
			return err
		}
		a.cleanupLog.Okf("cleanup completed")
		return nil
	}}
	cleanup.Flags().Bool("test-artifacts", false, "Clean local test artifacts")
	root.AddCommand(cleanup)

	runCmd := &cobra.Command{Use: "run", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		sensitive, _ := cmd.Flags().GetBool("sensitive")
		sudo, _ := cmd.Flags().GetBool("sudo")
		prompt, _ := cmd.Flags().GetString("prompt")
		return a.runCommand(args, sensitive, sudo, prompt)
	}}
	runCmd.Flags().Bool("sensitive", false, "Require confirmation before running")
	runCmd.Flags().Bool("sudo", false, "Run command with sudo")
	runCmd.Flags().String("prompt", "", "Custom confirmation prompt")
	root.AddCommand(runCmd)

	root.AddCommand(&cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(version.InstallerVersion)
		return nil
	}})

	workspaceCmd := &cobra.Command{Use: "workspace", Short: "Workspace helpers"}
	workspaceList := &cobra.Command{Use: "ls [paths...]", Aliases: []string{"list"}, Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		maxDepth, _ := cmd.Flags().GetInt("max-depth")
		return a.listWorkspaces(args, maxDepth)
	}}
	workspaceList.Flags().Int("max-depth", 4, "Max recursive search depth")
	workspaceCmd.AddCommand(workspaceList)
	root.AddCommand(workspaceCmd)

	modulesCmd := &cobra.Command{Use: "modules", Short: "Manage installable modules"}
	modulesInstall := &cobra.Command{Use: "install <module>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		mgr := modules.New(a.cfg.Modules, a.cfg.Mirrors, a.modulesLog, nil)
		report, err := mgr.Install(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		a.modulesLog.Okf("module install completed module=%s source=%s attempts=%d", report.Module, report.Source, len(report.Attempts))
		return nil
	}}
	modulesList := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		mgr := modules.New(a.cfg.Modules, a.cfg.Mirrors, a.modulesLog, nil)
		defs, err := mgr.List(cmd.Context())
		if err != nil {
			return err
		}
		for _, def := range defs {
			desc := strings.TrimSpace(def.Description)
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("%s\t%s\n", def.Name, desc)
		}
		return nil
	}}
	modulesCmd.AddCommand(modulesInstall, modulesList)
	root.AddCommand(modulesCmd)

	root.AddCommand(&cobra.Command{Use: instanceProc, Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		id := workspaceID
		displayName := defaultName
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			id = strings.TrimSpace(args[0])
		}
		if len(args) > 1 && strings.TrimSpace(args[1]) != "" {
			displayName = strings.TrimSpace(args[1])
		}
		a.runInstance(id, displayName)
		return nil
	}})

	return root
}

func (a *App) printHelp() {
	fmt.Println("MaiBot CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  maibot init                Initialize .maibot in current directory")
	fmt.Println("  maibot install             Alias of init")
	fmt.Println("  maibot create              Alias of init")
	fmt.Println("  maibot start               Start workspace")
	fmt.Println("  maibot stop                Stop workspace")
	fmt.Println("  maibot restart             Restart workspace")
	fmt.Println("  maibot status              Show workspace status")
	fmt.Println("  maibot workspace ls [paths...]   Discover workspaces under paths")
	fmt.Println("  maibot logs [--tail N]     Show workspace logs")
	fmt.Println("  maibot update              Update workspace")
	fmt.Println("  maibot upgrade             Upgrade maibot command")
	fmt.Println("  maibot modules install <name>  Install module by catalog name")
	fmt.Println("  maibot modules list        List configured/catalog modules")
	fmt.Println("  maibot service <action>    Manage workspace service")
	fmt.Println("  maibot run <cmd...>        Run developer command")
	fmt.Println("  maibot cleanup --test-artifacts  Clean local test artifacts")
	fmt.Println("  maibot version             Print version")
	fmt.Println("  maibot -C <dir> ...        Run command against another directory")
}

func (a *App) validateConfig() error {
	if strings.TrimSpace(a.cfg.Installer.Repo) == "" {
		return errors.New("installer.repo is empty in config")
	}
	if strings.TrimSpace(a.cfg.Installer.ReleaseChannel) == "" {
		return errors.New("installer.release_channel is empty in config")
	}
	if strings.TrimSpace(a.cfg.Installer.DataHome) == "" {
		return errors.New("installer.data_home is empty in config")
	}
	if strings.TrimSpace(a.cfg.Installer.InstanceTickInterval) == "" {
		return errors.New("installer.instance_tick_interval is empty in config")
	}
	if a.cfg.Updater.RequireSignature && strings.TrimSpace(a.cfg.Updater.MiniSignPublicKey) == "" {
		return errors.New("updater.minisign_public_key is empty while signature is required")
	}
	return nil
}
