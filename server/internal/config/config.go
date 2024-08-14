package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ark-network/ark/common"
	"github.com/spf13/viper"
)

type Config struct {
	Datadir               string
	WalletAddr            string
	RoundInterval         int64
	Port                  uint32
	EventDbType           string
	DbType                string
	DbDir                 string
	DbMigrationPath       string
	SchedulerType         string
	TxBuilderType         string
	BlockchainScannerType string
	NoTLS                 bool
	NoMacaroons           bool
	Network               common.Network
	LogLevel              int
	MinRelayFee           uint64
	RoundLifetime         int64
	UnilateralExitDelay   int64
	EsploraURL            string
	NeutrinoPeer          string
	TLSExtraIPs           []string
	TLSExtraDomains       []string
}

var (
	Datadir               = "DATADIR"
	WalletAddr            = "WALLET_ADDR"
	RoundInterval         = "ROUND_INTERVAL"
	Port                  = "PORT"
	EventDbType           = "EVENT_DB_TYPE"
	DbType                = "DB_TYPE"
	DbMigrationPath       = "DB_MIGRATION_PATH"
	SchedulerType         = "SCHEDULER_TYPE"
	TxBuilderType         = "TX_BUILDER_TYPE"
	BlockchainScannerType = "BC_SCANNER_TYPE"
	LogLevel              = "LOG_LEVEL"
	Network               = "NETWORK"
	MinRelayFee           = "MIN_RELAY_FEE"
	RoundLifetime         = "ROUND_LIFETIME"
	UnilateralExitDelay   = "UNILATERAL_EXIT_DELAY"
	EsploraURL            = "ESPLORA_URL"
	NeutrinoPeer          = "NEUTRINO_PEER"
	NoMacaroons           = "NO_MACAROONS"
	NoTLS                 = "NO_TLS"
	TLSExtraIP            = "TLS_EXTRA_IP"
	TLSExtraDomain        = "TLS_EXTRA_DOMAIN"

	defaultDatadir               = common.AppDataDir("arkd", false)
	defaultRoundInterval         = 5
	DefaultPort                  = 7070
	defaultWalletAddr            = "localhost:18000"
	defaultDbType                = "sqlite"
	defaultDbMigrationPath       = "file://internal/infrastructure/db/sqlite/migration"
	defaultEventDbType           = "badger"
	defaultSchedulerType         = "gocron"
	defaultTxBuilderType         = "covenant"
	defaultBlockchainScannerType = "ocean"
	defaultNetwork               = "liquid"
	defaultLogLevel              = 4
	defaultMinRelayFee           = 30 // 0.1 sat/vbyte on Liquid
	defaultRoundLifetime         = 604672
	defaultUnilateralExitDelay   = 1024
	defaultNoMacaroons           = false
	defaultNoTLS                 = false
)

func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("ARK")
	viper.AutomaticEnv()

	viper.SetDefault(Datadir, defaultDatadir)
	viper.SetDefault(Port, DefaultPort)
	viper.SetDefault(DbType, defaultDbType)
	viper.SetDefault(DbMigrationPath, defaultDbMigrationPath)
	viper.SetDefault(NoTLS, defaultNoTLS)
	viper.SetDefault(LogLevel, defaultLogLevel)
	viper.SetDefault(Network, defaultNetwork)
	viper.SetDefault(WalletAddr, defaultWalletAddr)
	viper.SetDefault(MinRelayFee, defaultMinRelayFee)
	viper.SetDefault(RoundInterval, defaultRoundInterval)
	viper.SetDefault(RoundLifetime, defaultRoundLifetime)
	viper.SetDefault(SchedulerType, defaultSchedulerType)
	viper.SetDefault(EventDbType, defaultEventDbType)
	viper.SetDefault(TxBuilderType, defaultTxBuilderType)
	viper.SetDefault(UnilateralExitDelay, defaultUnilateralExitDelay)
	viper.SetDefault(BlockchainScannerType, defaultBlockchainScannerType)
	viper.SetDefault(NoMacaroons, defaultNoMacaroons)

	net, err := getNetwork()
	if err != nil {
		return nil, fmt.Errorf("error while getting network: %s", err)
	}

	if err := initDatadir(); err != nil {
		return nil, fmt.Errorf("error while creating datadir: %s", err)
	}

	return &Config{
		Datadir:               viper.GetString(Datadir),
		WalletAddr:            viper.GetString(WalletAddr),
		RoundInterval:         viper.GetInt64(RoundInterval),
		Port:                  viper.GetUint32(Port),
		EventDbType:           viper.GetString(EventDbType),
		DbType:                viper.GetString(DbType),
		DbMigrationPath:       viper.GetString(DbMigrationPath),
		SchedulerType:         viper.GetString(SchedulerType),
		TxBuilderType:         viper.GetString(TxBuilderType),
		BlockchainScannerType: viper.GetString(BlockchainScannerType),
		NoTLS:                 viper.GetBool(NoTLS),
		DbDir:                 filepath.Join(viper.GetString(Datadir), "db"),
		LogLevel:              viper.GetInt(LogLevel),
		Network:               net,
		MinRelayFee:           viper.GetUint64(MinRelayFee),
		RoundLifetime:         viper.GetInt64(RoundLifetime),
		UnilateralExitDelay:   viper.GetInt64(UnilateralExitDelay),
		EsploraURL:            viper.GetString(EsploraURL),
		NeutrinoPeer:          viper.GetString(NeutrinoPeer),
		NoMacaroons:           viper.GetBool(NoMacaroons),
		TLSExtraIPs:           viper.GetStringSlice(TLSExtraIP),
		TLSExtraDomains:       viper.GetStringSlice(TLSExtraDomain),
	}, nil
}

func initDatadir() error {
	datadir := viper.GetString(Datadir)
	return makeDirectoryIfNotExists(datadir)
}

func makeDirectoryIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModeDir|0755)
	}
	return nil
}

func getNetwork() (common.Network, error) {
	switch strings.ToLower(viper.GetString(Network)) {
	case common.Liquid.Name:
		return common.Liquid, nil
	case common.LiquidTestNet.Name:
		return common.LiquidTestNet, nil
	case common.LiquidRegTest.Name:
		return common.LiquidRegTest, nil
	case common.Bitcoin.Name:
		return common.Bitcoin, nil
	case common.BitcoinTestNet.Name:
		return common.BitcoinTestNet, nil
	case common.BitcoinSigNet.Name:
		return common.BitcoinSigNet, nil
	case common.BitcoinRegTest.Name:
		return common.BitcoinRegTest, nil
	default:
		return common.Network{}, fmt.Errorf("unknown network %s", viper.GetString(Network))
	}
}
