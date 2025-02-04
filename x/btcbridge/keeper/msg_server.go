package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"github.com/sideprotocol/side/x/btcbridge/types"
)

type msgServer struct {
	Keeper
}

// SubmitBlockHeaders implements types.MsgServer.
func (m msgServer) SubmitBlockHeaders(goCtx context.Context, msg *types.MsgSubmitBlockHeaders) (*types.MsgSubmitBlockHeadersResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	// Set block headers
	err := m.SetBlockHeaders(ctx, msg.BlockHeaders)
	if err != nil {
		return nil, err
	}

	// Emit events
	// m.EmitEvent(
	// 	ctx,
	// 	msg.Sender,
	// 	sdk.Attribute{
	// 		Key:   types.AttributeKeyPoolCreator,
	// 		Value: msg.Sender,
	// 	},
	// )
	return &types.MsgSubmitBlockHeadersResponse{}, nil
}

// SubmitTransaction implements types.MsgServer.
// No Permission check required for this message
// Since everyone can submit a transaction to mint voucher tokens
// This message is usually sent by relayers
func (m msgServer) SubmitDepositTransaction(goCtx context.Context, msg *types.MsgSubmitDepositTransaction) (*types.MsgSubmitDepositTransactionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if err := msg.ValidateBasic(); err != nil {
		ctx.Logger().Error("Error validating basic", "error", err)
		return nil, err
	}

	txHash, recipient, err := m.ProcessBitcoinDepositTransaction(ctx, msg)
	if err != nil {
		ctx.Logger().Error("Error processing bitcoin deposit transaction", "error", err)
		return nil, err
	}

	// Emit Events
	m.EmitEvent(ctx, msg.Sender,
		sdk.NewAttribute("blockhash", msg.Blockhash),
		sdk.NewAttribute("txBytes", msg.TxBytes),
		sdk.NewAttribute("txid", txHash.String()),
		sdk.NewAttribute("recipient", recipient.EncodeAddress()),
	)

	return &types.MsgSubmitDepositTransactionResponse{}, nil
}

// SubmitTransaction implements types.MsgServer.
// No Permission check required for this message
// Since everyone can submit a transaction to mint voucher tokens
// This message is usually sent by relayers
func (m msgServer) SubmitWithdrawTransaction(goCtx context.Context, msg *types.MsgSubmitWithdrawTransaction) (*types.MsgSubmitWithdrawTransactionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if err := msg.ValidateBasic(); err != nil {
		ctx.Logger().Error("Error validating basic", "error", err)
		return nil, err
	}

	txHash, err := m.ProcessBitcoinWithdrawTransaction(ctx, msg)
	if err != nil {
		ctx.Logger().Error("Error processing bitcoin withdraw transaction", "error", err)
		return nil, err
	}

	// Emit Events
	m.EmitEvent(ctx, msg.Sender,
		sdk.NewAttribute("blockhash", msg.Blockhash),
		sdk.NewAttribute("txBytes", msg.TxBytes),
		sdk.NewAttribute("txid", txHash.String()),
	)

	return &types.MsgSubmitWithdrawTransactionResponse{}, nil
}

func (m msgServer) WithdrawToBitcoin(goCtx context.Context, msg *types.MsgWithdrawToBitcoin) (*types.MsgWithdrawToBitcoinResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)

	params := m.GetParams(ctx)

	sender := sdk.MustAccAddressFromBech32(msg.Sender)

	amount, err := sdk.ParseCoinNormalized(msg.Amount)
	if err != nil {
		return nil, err
	}

	networkFee := sdk.NewInt64Coin(params.BtcVoucherDenom, params.NetworkFee)

	if amount.Denom == params.BtcVoucherDenom {
		protocolFee := sdk.NewInt64Coin(params.BtcVoucherDenom, params.ProtocolFees.WithdrawFee)

		amount = amount.Sub(networkFee).Sub(protocolFee)
		if amount.Amount.Int64() < params.ProtocolLimits.BtcMinWithdraw || amount.Amount.Int64() > params.ProtocolLimits.BtcMaxWithdraw {
			return nil, types.ErrInvalidWithdrawAmount
		}

		if err := types.CheckOutputAmount(msg.Sender, amount.Amount.Int64()); err != nil {
			return nil, types.ErrInvalidWithdrawAmount
		}

		if err := m.bankKeeper.SendCoins(ctx, sender, sdk.MustAccAddressFromBech32(params.ProtocolFees.Collector), sdk.NewCoins(protocolFee)); err != nil {
			return nil, err
		}
	}

	req, err := m.Keeper.NewWithdrawRequest(ctx, msg.Sender, amount)
	if err != nil {
		return nil, err
	}

	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(amount, networkFee)); err != nil {
		return nil, err
	}

	if err := m.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(amount, networkFee)); err != nil {
		return nil, err
	}

	// Emit events
	m.EmitEvent(ctx, msg.Sender,
		sdk.NewAttribute("sequence", fmt.Sprintf("%d", req.Sequence)),
		sdk.NewAttribute("amount", msg.Amount),
	)

	return &types.MsgWithdrawToBitcoinResponse{}, nil
}

func (m msgServer) SubmitWithdrawStatus(goCtx context.Context, msg *types.MsgSubmitWithdrawStatus) (*types.MsgSubmitWithdrawStatusResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	exist := m.HasWithdrawRequest(ctx, msg.Sequence)
	if !exist {
		return nil, types.ErrWithdrawRequestNotExist
	}

	request := m.GetWithdrawRequest(ctx, msg.Sequence)
	request.Status = msg.Status
	m.SetWithdrawRequest(ctx, request)

	return &types.MsgSubmitWithdrawStatusResponse{}, nil
}

// InitiateDKG initiates the DKG request.
func (m msgServer) InitiateDKG(goCtx context.Context, msg *types.MsgInitiateDKG) (*types.MsgInitiateDKGResponse, error) {
	if m.authority != msg.Authority {
		return nil, errors.Wrapf(govtypes.ErrInvalidSigner, "invalid authority; expected %s, got %s", m.authority, msg.Authority)
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)

	req := &types.DKGRequest{
		Id:           m.Keeper.GetNextDKGRequestID(ctx),
		Participants: msg.Participants,
		Threshold:    msg.Threshold,
		Expiration:   m.Keeper.GetDKGRequestExpirationTime(ctx),
		Status:       types.DKGRequestStatus_DKG_REQUEST_STATUS_PENDING,
	}

	m.Keeper.SetDKGRequest(ctx, req)
	m.Keeper.SetDKGRequestID(ctx, req.Id)

	// Emit events
	m.EmitEvent(ctx, msg.Authority,
		sdk.NewAttribute("id", fmt.Sprintf("%d", req.Id)),
		sdk.NewAttribute("expiration", req.Expiration.String()),
	)

	return &types.MsgInitiateDKGResponse{}, nil
}

// CompleteDKG initiates the DKG request.
func (m msgServer) CompleteDKG(goCtx context.Context, msg *types.MsgCompleteDKG) (*types.MsgCompleteDKGResponse, error) {
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)

	req := &types.DKGCompletionRequest{
		Id:     msg.Id,
		Sender: msg.Sender,
		Vaults: msg.Vaults,
	}

	if err := m.Keeper.CompleteDKG(ctx, req); err != nil {
		return nil, err
	}

	// Emit events
	m.EmitEvent(ctx, msg.Sender,
		sdk.NewAttribute("id", fmt.Sprintf("%d", msg.Id)),
	)

	return &types.MsgCompleteDKGResponse{}, nil
}

// UpdateParams updates the module params.
func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if m.authority != msg.Authority {
		return nil, errors.Wrapf(govtypes.ErrInvalidSigner, "invalid authority; expected %s, got %s", m.authority, msg.Authority)
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	m.SetParams(ctx, msg.Params)

	return &types.MsgUpdateParamsResponse{}, nil
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}
