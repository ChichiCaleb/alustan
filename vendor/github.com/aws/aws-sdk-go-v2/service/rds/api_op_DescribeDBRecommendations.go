// Code generated by smithy-go-codegen DO NOT EDIT.

package rds

import (
	"context"
	"fmt"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"time"
)

// Describes the recommendations to resolve the issues for your DB instances, DB
// clusters, and DB parameter groups.
func (c *Client) DescribeDBRecommendations(ctx context.Context, params *DescribeDBRecommendationsInput, optFns ...func(*Options)) (*DescribeDBRecommendationsOutput, error) {
	if params == nil {
		params = &DescribeDBRecommendationsInput{}
	}

	result, metadata, err := c.invokeOperation(ctx, "DescribeDBRecommendations", params, optFns, c.addOperationDescribeDBRecommendationsMiddlewares)
	if err != nil {
		return nil, err
	}

	out := result.(*DescribeDBRecommendationsOutput)
	out.ResultMetadata = metadata
	return out, nil
}

type DescribeDBRecommendationsInput struct {

	// A filter that specifies one or more recommendations to describe.
	//
	// Supported Filters:
	//
	//   - recommendation-id - Accepts a list of recommendation identifiers. The
	//   results list only includes the recommendations whose identifier is one of the
	//   specified filter values.
	//
	//   - status - Accepts a list of recommendation statuses.
	//
	// Valid values:
	//
	//   - active - The recommendations which are ready for you to apply.
	//
	//   - pending - The applied or scheduled recommendations which are in progress.
	//
	//   - resolved - The recommendations which are completed.
	//
	//   - dismissed - The recommendations that you dismissed.
	//
	// The results list only includes the recommendations whose status is one of the
	//   specified filter values.
	//
	//   - severity - Accepts a list of recommendation severities. The results list
	//   only includes the recommendations whose severity is one of the specified filter
	//   values.
	//
	// Valid values:
	//
	//   - high
	//
	//   - medium
	//
	//   - low
	//
	//   - informational
	//
	//   - type-id - Accepts a list of recommendation type identifiers. The results
	//   list only includes the recommendations whose type is one of the specified filter
	//   values.
	//
	//   - dbi-resource-id - Accepts a list of database resource identifiers. The
	//   results list only includes the recommendations that generated for the specified
	//   databases.
	//
	//   - cluster-resource-id - Accepts a list of cluster resource identifiers. The
	//   results list only includes the recommendations that generated for the specified
	//   clusters.
	//
	//   - pg-arn - Accepts a list of parameter group ARNs. The results list only
	//   includes the recommendations that generated for the specified parameter groups.
	//
	//   - cluster-pg-arn - Accepts a list of cluster parameter group ARNs. The results
	//   list only includes the recommendations that generated for the specified cluster
	//   parameter groups.
	Filters []types.Filter

	// A filter to include only the recommendations that were updated after this
	// specified time.
	LastUpdatedAfter *time.Time

	// A filter to include only the recommendations that were updated before this
	// specified time.
	LastUpdatedBefore *time.Time

	// The language that you choose to return the list of recommendations.
	//
	// Valid values:
	//
	//   - en
	//
	//   - en_UK
	//
	//   - de
	//
	//   - es
	//
	//   - fr
	//
	//   - id
	//
	//   - it
	//
	//   - ja
	//
	//   - ko
	//
	//   - pt_BR
	//
	//   - zh_TW
	//
	//   - zh_CN
	Locale *string

	// An optional pagination token provided by a previous DescribeDBRecommendations
	// request. If this parameter is specified, the response includes only records
	// beyond the marker, up to the value specified by MaxRecords .
	Marker *string

	// The maximum number of recommendations to include in the response. If more
	// records exist than the specified MaxRecords value, a pagination token called a
	// marker is included in the response so that you can retrieve the remaining
	// results.
	MaxRecords *int32

	noSmithyDocumentSerde
}

type DescribeDBRecommendationsOutput struct {

	// A list of recommendations which is returned from DescribeDBRecommendations API
	// request.
	DBRecommendations []types.DBRecommendation

	// An optional pagination token provided by a previous DBRecommendationsMessage
	// request. This token can be used later in a DescribeDBRecomendations request.
	Marker *string

	// Metadata pertaining to the operation's result.
	ResultMetadata middleware.Metadata

	noSmithyDocumentSerde
}

func (c *Client) addOperationDescribeDBRecommendationsMiddlewares(stack *middleware.Stack, options Options) (err error) {
	if err := stack.Serialize.Add(&setOperationInputMiddleware{}, middleware.After); err != nil {
		return err
	}
	err = stack.Serialize.Add(&awsAwsquery_serializeOpDescribeDBRecommendations{}, middleware.After)
	if err != nil {
		return err
	}
	err = stack.Deserialize.Add(&awsAwsquery_deserializeOpDescribeDBRecommendations{}, middleware.After)
	if err != nil {
		return err
	}
	if err := addProtocolFinalizerMiddlewares(stack, options, "DescribeDBRecommendations"); err != nil {
		return fmt.Errorf("add protocol finalizers: %v", err)
	}

	if err = addlegacyEndpointContextSetter(stack, options); err != nil {
		return err
	}
	if err = addSetLoggerMiddleware(stack, options); err != nil {
		return err
	}
	if err = addClientRequestID(stack); err != nil {
		return err
	}
	if err = addComputeContentLength(stack); err != nil {
		return err
	}
	if err = addResolveEndpointMiddleware(stack, options); err != nil {
		return err
	}
	if err = addComputePayloadSHA256(stack); err != nil {
		return err
	}
	if err = addRetry(stack, options); err != nil {
		return err
	}
	if err = addRawResponseToMetadata(stack); err != nil {
		return err
	}
	if err = addRecordResponseTiming(stack); err != nil {
		return err
	}
	if err = addClientUserAgent(stack, options); err != nil {
		return err
	}
	if err = smithyhttp.AddErrorCloseResponseBodyMiddleware(stack); err != nil {
		return err
	}
	if err = smithyhttp.AddCloseResponseBodyMiddleware(stack); err != nil {
		return err
	}
	if err = addSetLegacyContextSigningOptionsMiddleware(stack); err != nil {
		return err
	}
	if err = addTimeOffsetBuild(stack, c); err != nil {
		return err
	}
	if err = addUserAgentRetryMode(stack, options); err != nil {
		return err
	}
	if err = addOpDescribeDBRecommendationsValidationMiddleware(stack); err != nil {
		return err
	}
	if err = stack.Initialize.Add(newServiceMetadataMiddleware_opDescribeDBRecommendations(options.Region), middleware.Before); err != nil {
		return err
	}
	if err = addRecursionDetection(stack); err != nil {
		return err
	}
	if err = addRequestIDRetrieverMiddleware(stack); err != nil {
		return err
	}
	if err = addResponseErrorMiddleware(stack); err != nil {
		return err
	}
	if err = addRequestResponseLogging(stack, options); err != nil {
		return err
	}
	if err = addDisableHTTPSMiddleware(stack, options); err != nil {
		return err
	}
	return nil
}

// DescribeDBRecommendationsPaginatorOptions is the paginator options for
// DescribeDBRecommendations
type DescribeDBRecommendationsPaginatorOptions struct {
	// The maximum number of recommendations to include in the response. If more
	// records exist than the specified MaxRecords value, a pagination token called a
	// marker is included in the response so that you can retrieve the remaining
	// results.
	Limit int32

	// Set to true if pagination should stop if the service returns a pagination token
	// that matches the most recent token provided to the service.
	StopOnDuplicateToken bool
}

// DescribeDBRecommendationsPaginator is a paginator for DescribeDBRecommendations
type DescribeDBRecommendationsPaginator struct {
	options   DescribeDBRecommendationsPaginatorOptions
	client    DescribeDBRecommendationsAPIClient
	params    *DescribeDBRecommendationsInput
	nextToken *string
	firstPage bool
}

// NewDescribeDBRecommendationsPaginator returns a new
// DescribeDBRecommendationsPaginator
func NewDescribeDBRecommendationsPaginator(client DescribeDBRecommendationsAPIClient, params *DescribeDBRecommendationsInput, optFns ...func(*DescribeDBRecommendationsPaginatorOptions)) *DescribeDBRecommendationsPaginator {
	if params == nil {
		params = &DescribeDBRecommendationsInput{}
	}

	options := DescribeDBRecommendationsPaginatorOptions{}
	if params.MaxRecords != nil {
		options.Limit = *params.MaxRecords
	}

	for _, fn := range optFns {
		fn(&options)
	}

	return &DescribeDBRecommendationsPaginator{
		options:   options,
		client:    client,
		params:    params,
		firstPage: true,
		nextToken: params.Marker,
	}
}

// HasMorePages returns a boolean indicating whether more pages are available
func (p *DescribeDBRecommendationsPaginator) HasMorePages() bool {
	return p.firstPage || (p.nextToken != nil && len(*p.nextToken) != 0)
}

// NextPage retrieves the next DescribeDBRecommendations page.
func (p *DescribeDBRecommendationsPaginator) NextPage(ctx context.Context, optFns ...func(*Options)) (*DescribeDBRecommendationsOutput, error) {
	if !p.HasMorePages() {
		return nil, fmt.Errorf("no more pages available")
	}

	params := *p.params
	params.Marker = p.nextToken

	var limit *int32
	if p.options.Limit > 0 {
		limit = &p.options.Limit
	}
	params.MaxRecords = limit

	optFns = append([]func(*Options){
		addIsPaginatorUserAgent,
	}, optFns...)
	result, err := p.client.DescribeDBRecommendations(ctx, &params, optFns...)
	if err != nil {
		return nil, err
	}
	p.firstPage = false

	prevToken := p.nextToken
	p.nextToken = result.Marker

	if p.options.StopOnDuplicateToken &&
		prevToken != nil &&
		p.nextToken != nil &&
		*prevToken == *p.nextToken {
		p.nextToken = nil
	}

	return result, nil
}

// DescribeDBRecommendationsAPIClient is a client that implements the
// DescribeDBRecommendations operation.
type DescribeDBRecommendationsAPIClient interface {
	DescribeDBRecommendations(context.Context, *DescribeDBRecommendationsInput, ...func(*Options)) (*DescribeDBRecommendationsOutput, error)
}

var _ DescribeDBRecommendationsAPIClient = (*Client)(nil)

func newServiceMetadataMiddleware_opDescribeDBRecommendations(region string) *awsmiddleware.RegisterServiceMetadata {
	return &awsmiddleware.RegisterServiceMetadata{
		Region:        region,
		ServiceID:     ServiceID,
		OperationName: "DescribeDBRecommendations",
	}
}
