package appconfig

import (
	"fmt"
	"strings"

	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/internal/core/application"
	"github.com/ark-network/ark/internal/core/ports"
	"github.com/ark-network/ark/internal/infrastructure/db"
	scheduler "github.com/ark-network/ark/internal/infrastructure/scheduler/gocron"
	txbuilder "github.com/ark-network/ark/internal/infrastructure/tx-builder/covenant"
	cltxbuilder "github.com/ark-network/ark/internal/infrastructure/tx-builder/covenantless"
	btcwallet "github.com/ark-network/ark/internal/infrastructure/wallet/btc-embedded"
	liquidwallet "github.com/ark-network/ark/internal/infrastructure/wallet/liquid-standalone"

	log "github.com/sirupsen/logrus"
)

const minAllowedSequence = 512

var (
	supportedDbs = supportedType{
		"badger": {},
	}
	supportedSchedulers = supportedType{
		"gocron": {},
	}
	supportedTxBuilders = supportedType{
		"covenant":     {},
		"covenantless": {},
	}
	supportedScanners = supportedType{
		"ocean":     {},
		"btcwallet": {},
	}
)

type Config struct {
	DbType                string
	DbDir                 string
	RoundInterval         int64
	Network               common.Network
	SchedulerType         string
	TxBuilderType         string
	BlockchainScannerType string
	WalletAddr            string
	MinRelayFee           uint64
	RoundLifetime         int64
	UnilateralExitDelay   int64

	EsploraURL   string
	NeutrinoPeer string

	repo      ports.RepoManager
	svc       application.Service
	adminSvc  application.AdminService
	wallet    ports.WalletService
	txBuilder ports.TxBuilder
	scanner   ports.BlockchainScanner
	scheduler ports.SchedulerService
}

func (c *Config) Validate() error {
	if !supportedDbs.supports(c.DbType) {
		return fmt.Errorf("db type not supported, please select one of: %s", supportedDbs)
	}
	if !supportedSchedulers.supports(c.SchedulerType) {
		return fmt.Errorf("scheduler type not supported, please select one of: %s", supportedSchedulers)
	}
	if !supportedTxBuilders.supports(c.TxBuilderType) {
		return fmt.Errorf("tx builder type not supported, please select one of: %s", supportedTxBuilders)
	}
	if !supportedScanners.supports(c.BlockchainScannerType) {
		return fmt.Errorf("blockchain scanner type not supported, please select one of: %s", supportedScanners)
	}
	if c.RoundInterval < 2 {
		return fmt.Errorf("invalid round interval, must be at least 2 seconds")
	}
	if c.Network.Name != "liquid" && c.Network.Name != "testnet" && c.Network.Name != "regtest" {
		return fmt.Errorf("invalid network, must be liquid, testnet or regtest")
	}
	if len(c.WalletAddr) <= 0 {
		return fmt.Errorf("missing onchain wallet address")
	}
	if c.MinRelayFee < 30 {
		return fmt.Errorf("invalid min relay fee, must be at least 30 sats")
	}
	// round life time must be a multiple of 512
	if c.RoundLifetime < minAllowedSequence {
		return fmt.Errorf(
			"invalid round lifetime, must be a at least %d", minAllowedSequence,
		)
	}

	if c.UnilateralExitDelay < minAllowedSequence {
		return fmt.Errorf(
			"invalid unilateral exit delay, must at least %d", minAllowedSequence,
		)
	}

	if c.RoundLifetime%minAllowedSequence != 0 {
		c.RoundLifetime -= c.RoundLifetime % minAllowedSequence
		log.Infof(
			"round lifetime must be a multiple of %d, rounded to %d",
			minAllowedSequence, c.RoundLifetime,
		)
	}

	if c.UnilateralExitDelay%minAllowedSequence != 0 {
		c.UnilateralExitDelay -= c.UnilateralExitDelay % minAllowedSequence
		log.Infof(
			"unilateral exit delay must be a multiple of %d, rounded to %d",
			minAllowedSequence, c.UnilateralExitDelay,
		)
	}

	if err := c.repoManager(); err != nil {
		return err
	}
	if err := c.walletService(); err != nil {
		return fmt.Errorf("failed to connect to wallet: %s", err)
	}
	if err := c.txBuilderService(); err != nil {
		return err
	}
	if err := c.scannerService(); err != nil {
		return err
	}
	if err := c.schedulerService(); err != nil {
		return err
	}
	if err := c.appService(); err != nil {
		return err
	}
	if err := c.adminService(); err != nil {
		return err
	}
	return nil
}

func (c *Config) AppService() application.Service {
	return c.svc
}

func (c *Config) AdminService() application.AdminService {
	return c.adminSvc
}

func (c *Config) repoManager() error {
	var svc ports.RepoManager
	var err error
	switch c.DbType {
	case "badger":
		logger := log.New()
		svc, err = db.NewService(db.ServiceConfig{
			EventStoreType: c.DbType,
			RoundStoreType: c.DbType,
			VtxoStoreType:  c.DbType,

			EventStoreConfig: []interface{}{c.DbDir, logger},
			RoundStoreConfig: []interface{}{c.DbDir, logger},
			VtxoStoreConfig:  []interface{}{c.DbDir, logger},
		})
	default:
		return fmt.Errorf("unknown db type")
	}
	if err != nil {
		return err
	}

	c.repo = svc
	return nil
}

func (c *Config) walletService() error {
	if common.IsLiquid(c.Network) {
		svc, err := liquidwallet.NewService(c.WalletAddr)
		if err != nil {
			return err
		}

		c.wallet = svc
		return nil
	}

	if len(c.EsploraURL) == 0 {
		return fmt.Errorf("missing esplora url, covenant-less ark requires ARK_ESPLORA_URL to be set")
	}

	svc, err := btcwallet.NewService(btcwallet.WalletConfig{
		Datadir: c.DbDir,
		// TODO let the operator set the passwords
		Password:   []byte("password"),
		Network:    c.Network,
		EsploraURL: c.EsploraURL,
	},
		btcwallet.WithNeutrino(c.NeutrinoPeer),
	)
	if err != nil {
		return err
	}

	c.wallet = svc
	return nil
}

func (c *Config) txBuilderService() error {
	var svc ports.TxBuilder
	var err error
	switch c.TxBuilderType {
	case "covenant":
		svc = txbuilder.NewTxBuilder(
			c.wallet, c.Network, c.RoundLifetime, c.UnilateralExitDelay,
		)
	case "covenantless":
		svc = cltxbuilder.NewTxBuilder(
			c.wallet, c.Network, c.RoundLifetime, c.UnilateralExitDelay,
		)
	default:
		err = fmt.Errorf("unknown tx builder type")
	}
	if err != nil {
		return err
	}

	c.txBuilder = svc
	return nil
}

func (c *Config) scannerService() error {
	var svc ports.BlockchainScanner
	switch c.BlockchainScannerType {
	default:
		svc = c.wallet
	}

	c.scanner = svc
	return nil
}

func (c *Config) schedulerService() error {
	var svc ports.SchedulerService
	var err error
	switch c.SchedulerType {
	case "gocron":
		svc = scheduler.NewScheduler()
	default:
		err = fmt.Errorf("unknown scheduler type")
	}
	if err != nil {
		return err
	}

	c.scheduler = svc
	return nil
}

func (c *Config) appService() error {
	if common.IsLiquid(c.Network) {
		svc, err := application.NewCovenantService(
			c.Network, c.RoundInterval, c.RoundLifetime, c.UnilateralExitDelay,
			c.MinRelayFee, c.wallet, c.repo, c.txBuilder, c.scanner, c.scheduler,
		)
		if err != nil {
			return err
		}

		c.svc = svc
		return nil
	}

	svc, err := application.NewCovenantlessService(
		c.Network, c.RoundInterval, c.RoundLifetime, c.UnilateralExitDelay,
		c.MinRelayFee, c.wallet, c.repo, c.txBuilder, c.scanner, c.scheduler,
	)
	if err != nil {
		return err
	}

	c.svc = svc
	return nil
}

func (c *Config) adminService() error {
	c.adminSvc = application.NewAdminService(c.wallet, c.repo, c.txBuilder)
	return nil
}

type supportedType map[string]struct{}

func (t supportedType) String() string {
	types := make([]string, 0, len(t))
	for tt := range t {
		types = append(types, tt)
	}
	return strings.Join(types, " | ")
}

func (t supportedType) supports(typeStr string) bool {
	_, ok := t[typeStr]
	return ok
}
