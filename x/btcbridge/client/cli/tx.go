package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"

	// "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/sideprotocol/side/x/btcbridge/types"
)

var DefaultRelativePacketTimeoutTimestamp = uint64((time.Duration(10) * time.Minute).Nanoseconds())

const (
	// flagPacketTimeoutTimestamp = "packet-timeout-timestamp"
	listSeparator = ","
)

// GetTxCmd returns the transaction commands for this module
func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      fmt.Sprintf("%s transactions subcommands", types.ModuleName),
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(CmdSubmitBlocks())
	cmd.AddCommand(CmdWithdrawToBitcoin())
	cmd.AddCommand(CmdCompleteDKG())

	return cmd
}

func CmdSubmitBlocks() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-blocks [file-path-to-block-headers.json]",
		Short: "Submit Bitcoin block headers to the chain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			// read the block headers from the file
			blockHeaders, err := readBlockHeadersFromFile(args[0])
			if err != nil {
				return err
			}

			msg := types.NewMsgSubmitBlockHeaders(
				clientCtx.GetFromAddress().String(),
				blockHeaders,
			)
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

// Withdraw To Bitcoin
func CmdWithdrawToBitcoin() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "withdraw [amount]",
		Short: "Withdraw asset to the given sender",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			_, err = sdk.ParseCoinsNormalized(args[0])
			if err != nil {
				return fmt.Errorf("invalid amount")
			}

			msg := types.NewMsgWithdrawToBitcoin(
				clientCtx.GetFromAddress().String(),
				args[0],
			)

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

// Complete DKG
func CmdCompleteDKG() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "complete-dkg [id] [vaults] [validator-address] [signature]",
		Short: "Complete dkg request with new vaults",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return err
			}

			vaults := strings.Split(args[1], listSeparator)

			msg := types.NewMsgCompleteDKG(
				clientCtx.GetFromAddress().String(),
				id,
				vaults,
				args[2],
				args[3],
			)

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

// readBlockHeadersFromFile reads the block headers from the file
func readBlockHeadersFromFile(filePath string) ([]*types.BlockHeader, error) {
	// read the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// read the block headers from the file
	var blockHeaders []*types.BlockHeader
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&blockHeaders); err != nil {
		return nil, err
	}
	return blockHeaders, nil
}
