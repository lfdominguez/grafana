package loganalytics

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/grafana/pkg/tsdb/azuremonitor/kinds/dataquery"
	"github.com/grafana/grafana/pkg/tsdb/azuremonitor/macros"
	"github.com/grafana/grafana/pkg/tsdb/azuremonitor/types"
)

func (e *AzureLogAnalyticsDatasource) ResourceRequest(rw http.ResponseWriter, req *http.Request, cli *http.Client) (http.ResponseWriter, error) {
	return e.Proxy.Do(rw, req, cli)
}

// executeTimeSeriesQuery does the following:
// 1. build the AzureMonitor url and querystring for each query
// 2. executes each query by calling the Azure Monitor API
// 3. parses the responses for each query into data frames
func (e *AzureLogAnalyticsDatasource) ExecuteTimeSeriesQuery(ctx context.Context, originalQueries []backend.DataQuery, dsInfo types.DatasourceInfo, client *http.Client, url string) (*backend.QueryDataResponse, error) {
	result := backend.NewQueryDataResponse()
	queries, err := e.buildQueries(ctx, originalQueries, dsInfo)
	if err != nil {
		return nil, err
	}

	for _, query := range queries {
		res, err := e.executeQuery(ctx, query, dsInfo, client, url)
		if err != nil {
			result.Responses[query.RefID] = backend.DataResponse{Error: err}
			continue
		}
		result.Responses[query.RefID] = *res
	}

	return result, nil
}

func buildLogAnalyticsQuery(query backend.DataQuery, dsInfo types.DatasourceInfo, appInsightsRegExp *regexp.Regexp) (*AzureLogAnalyticsQuery, error) {
	queryJSONModel := types.LogJSONQuery{}
	err := json.Unmarshal(query.JSON, &queryJSONModel)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the Azure Log Analytics query object from JSON: %w", err)
	}
	var queryString string
	appInsightsQuery := false
	dashboardTime := false
	timeColumn := ""
	azureLogAnalyticsTarget := queryJSONModel.AzureLogAnalytics

	resultFormat := ParseResultFormat(azureLogAnalyticsTarget.ResultFormat, dataquery.AzureQueryTypeAzureLogAnalytics)

	resources, resourceOrWorkspace := retrieveResources(azureLogAnalyticsTarget)
	appInsightsQuery = appInsightsRegExp.Match([]byte(resourceOrWorkspace))

	if azureLogAnalyticsTarget.Query != nil {
		queryString = *azureLogAnalyticsTarget.Query
	}

	if azureLogAnalyticsTarget.DashboardTime != nil {
		dashboardTime = *azureLogAnalyticsTarget.DashboardTime
		if dashboardTime {
			if azureLogAnalyticsTarget.TimeColumn != nil {
				timeColumn = *azureLogAnalyticsTarget.TimeColumn
			} else {
				// Final fallback to TimeGenerated if no column is provided
				timeColumn = "TimeGenerated"
			}
		}
	}

	apiURL := getApiURL(resourceOrWorkspace, appInsightsQuery)

	rawQuery, err := macros.KqlInterpolate(query, dsInfo, queryString, "TimeGenerated")
	if err != nil {
		return nil, err
	}

	return &AzureLogAnalyticsQuery{
		RefID:            query.RefID,
		ResultFormat:     resultFormat,
		URL:              apiURL,
		JSON:             query.JSON,
		TimeRange:        query.TimeRange,
		Query:            rawQuery,
		Resources:        resources,
		QueryType:        dataquery.AzureQueryType(query.QueryType),
		AppInsightsQuery: appInsightsQuery,
		DashboardTime:    dashboardTime,
		TimeColumn:       timeColumn,
	}, nil
}

func (e *AzureLogAnalyticsDatasource) buildQueries(ctx context.Context, queries []backend.DataQuery, dsInfo types.DatasourceInfo) ([]*AzureLogAnalyticsQuery, error) {
	azureLogAnalyticsQueries := []*AzureLogAnalyticsQuery{}
	appInsightsRegExp, err := regexp.Compile("providers/Microsoft.Insights/components")
	if err != nil {
		return nil, fmt.Errorf("failed to compile Application Insights regex")
	}

	for _, query := range queries {
		if query.QueryType == string(dataquery.AzureQueryTypeAzureLogAnalytics) {
			azureLogAnalyticsQuery, err := buildLogAnalyticsQuery(query, dsInfo, appInsightsRegExp)
			if err != nil {
				return nil, fmt.Errorf("failed to build azure log analytics query: %w", err)
			}
			azureLogAnalyticsQueries = append(azureLogAnalyticsQueries, azureLogAnalyticsQuery)
		}

		if query.QueryType == string(dataquery.AzureQueryTypeAzureTraces) {
			azureAppInsightsQuery, err := buildAppInsightsQuery(ctx, query, dsInfo, appInsightsRegExp)
			if err != nil {
				return nil, fmt.Errorf("failed to build azure application insights query: %w", err)
			}
			azureLogAnalyticsQueries = append(azureLogAnalyticsQueries, azureAppInsightsQuery)
		}
	}

	return azureLogAnalyticsQueries, nil
}

func (e *AzureLogAnalyticsDatasource) executeQuery(ctx context.Context, query *AzureLogAnalyticsQuery, dsInfo types.DatasourceInfo, client *http.Client, url string) (*backend.DataResponse, error) {
	// If azureLogAnalyticsSameAs is defined and set to false, return an error
	if sameAs, ok := dsInfo.JSONData["azureLogAnalyticsSameAs"]; ok && !sameAs.(bool) {
		return nil, fmt.Errorf("credentials for Log Analytics are no longer supported. Go to the data source configuration to update Azure Monitor credentials")
	}

	queryJSONModel := dataquery.AzureMonitorQuery{}
	err := json.Unmarshal(query.JSON, &queryJSONModel)
	if err != nil {
		return nil, err
	}

	if query.QueryType == dataquery.AzureQueryTypeAzureTraces {
		if query.ResultFormat == dataquery.ResultFormatTrace && query.Query == "" {
			return nil, fmt.Errorf("cannot visualise trace events using the trace visualiser")
		}
	}

	req, err := e.createRequest(ctx, url, query)
	if err != nil {
		return nil, err
	}

	_, span := tracing.DefaultTracer().Start(ctx, "azure log analytics query", trace.WithAttributes(
		attribute.String("target", query.Query),
		attribute.Int64("from", query.TimeRange.From.UnixNano()/int64(time.Millisecond)),
		attribute.Int64("until", query.TimeRange.To.UnixNano()/int64(time.Millisecond)),
		attribute.Int64("datasource_id", dsInfo.DatasourceID),
		attribute.Int64("org_id", dsInfo.OrgID),
	))
	defer span.End()

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			e.Logger.Warn("Failed to close response body", "err", err)
		}
	}()

	logResponse, err := e.unmarshalResponse(res)
	if err != nil {
		return nil, err
	}

	t, err := logResponse.GetPrimaryResultTable()
	if err != nil {
		return nil, err
	}

	frame, err := ResponseTableToFrame(t, query.RefID, query.Query, query.QueryType, query.ResultFormat)
	if err != nil {
		return nil, err
	}
	frame = appendErrorNotice(frame, logResponse.Error)
	if frame == nil {
		dataResponse := backend.DataResponse{}
		return &dataResponse, nil
	}

	queryUrl, err := getQueryUrl(query.Query, query.Resources, dsInfo.Routes["Azure Portal"].URL, query.TimeRange)
	if err != nil {
		return nil, err
	}

	if query.QueryType == dataquery.AzureQueryTypeAzureTraces && query.ResultFormat == dataquery.ResultFormatTrace {
		frame.Meta.PreferredVisualization = data.VisTypeTrace
	}

	if query.ResultFormat == dataquery.ResultFormatTable {
		frame.Meta.PreferredVisualization = data.VisTypeTable
	}

	if query.ResultFormat == dataquery.ResultFormatLogs {
		frame.Meta.PreferredVisualization = data.VisTypeLogs
		frame.Meta.Custom = &LogAnalyticsMeta{
			ColumnTypes:     frame.Meta.Custom.(*LogAnalyticsMeta).ColumnTypes,
			AzurePortalLink: queryUrl,
		}
	}

	if query.ResultFormat == dataquery.ResultFormatTimeSeries {
		tsSchema := frame.TimeSeriesSchema()
		if tsSchema.Type == data.TimeSeriesTypeLong {
			wideFrame, err := data.LongToWide(frame, nil)
			if err == nil {
				frame = wideFrame
			} else {
				frame.AppendNotices(data.Notice{Severity: data.NoticeSeverityWarning, Text: "could not convert frame to time series, returning raw table: " + err.Error()})
			}
		}
	}

	// Use the parent span query for the parent span data link
	err = addDataLinksToFields(query, dsInfo.Routes["Azure Portal"].URL, frame, dsInfo, queryUrl)
	if err != nil {
		return nil, err
	}

	dataResponse := backend.DataResponse{Frames: data.Frames{frame}}
	return &dataResponse, nil
}

func addDataLinksToFields(query *AzureLogAnalyticsQuery, azurePortalBaseUrl string, frame *data.Frame, dsInfo types.DatasourceInfo, queryUrl string) error {
	if query.QueryType == dataquery.AzureQueryTypeAzureTraces {
		err := addTraceDataLinksToFields(query, azurePortalBaseUrl, frame, dsInfo)
		if err != nil {
			return err
		}

		return nil
	}

	if query.ResultFormat == dataquery.ResultFormatLogs {
		return nil
	}

	AddConfigLinks(*frame, queryUrl, nil)

	return nil
}

func addTraceDataLinksToFields(query *AzureLogAnalyticsQuery, azurePortalBaseUrl string, frame *data.Frame, dsInfo types.DatasourceInfo) error {
	tracesUrl, err := getTracesQueryUrl(query.Resources, azurePortalBaseUrl)
	if err != nil {
		return err
	}

	queryJSONModel := dataquery.AzureMonitorQuery{}
	err = json.Unmarshal(query.JSON, &queryJSONModel)
	if err != nil {
		return err
	}

	traceIdVariable := "${__data.fields.traceID}"
	resultFormat := dataquery.ResultFormatTrace
	queryJSONModel.AzureTraces.ResultFormat = &resultFormat
	queryJSONModel.AzureTraces.Query = &query.TraceExploreQuery
	if queryJSONModel.AzureTraces.OperationId == nil || *queryJSONModel.AzureTraces.OperationId == "" {
		queryJSONModel.AzureTraces.OperationId = &traceIdVariable
	}

	logsQueryType := string(dataquery.AzureQueryTypeAzureLogAnalytics)
	logsJSONModel := dataquery.AzureMonitorQuery{
		QueryType: &logsQueryType,
		AzureLogAnalytics: &dataquery.AzureLogsQuery{
			Query:     &query.TraceLogsExploreQuery,
			Resources: []string{queryJSONModel.AzureTraces.Resources[0]},
		},
	}

	if query.ResultFormat == dataquery.ResultFormatTable {
		AddCustomDataLink(*frame, data.DataLink{
			Title: "Explore Trace: ${__data.fields.traceID}",
			URL:   "",
			Internal: &data.InternalDataLink{
				DatasourceUID:  dsInfo.DatasourceUID,
				DatasourceName: dsInfo.DatasourceName,
				Query:          queryJSONModel,
			},
		})

		queryJSONModel.AzureTraces.Query = &query.TraceParentExploreQuery
		AddCustomDataLink(*frame, data.DataLink{
			Title: "Explore Parent Span: ${__data.fields.parentSpanID}",
			URL:   "",
			Internal: &data.InternalDataLink{
				DatasourceUID:  dsInfo.DatasourceUID,
				DatasourceName: dsInfo.DatasourceName,
				Query:          queryJSONModel,
			},
		})

		linkTitle := "Explore Trace in Azure Portal"
		AddConfigLinks(*frame, tracesUrl, &linkTitle)
	}

	AddCustomDataLink(*frame, data.DataLink{
		Title: "Explore Trace Logs",
		URL:   "",
		Internal: &data.InternalDataLink{
			DatasourceUID:  dsInfo.DatasourceUID,
			DatasourceName: dsInfo.DatasourceName,
			Query:          logsJSONModel,
		},
	})

	return nil
}

func appendErrorNotice(frame *data.Frame, err *AzureLogAnalyticsAPIError) *data.Frame {
	if err == nil {
		return frame
	}
	if frame == nil {
		frame = &data.Frame{}
	}
	frame.AppendNotices(apiErrorToNotice(err))
	return frame
}

func (e *AzureLogAnalyticsDatasource) createRequest(ctx context.Context, queryURL string, query *AzureLogAnalyticsQuery) (*http.Request, error) {
	body := map[string]interface{}{
		"query": query.Query,
	}

	if query.DashboardTime {
		from := query.TimeRange.From.Format(time.RFC3339)
		to := query.TimeRange.To.Format(time.RFC3339)
		timespan := fmt.Sprintf("%s/%s", from, to)
		body["timespan"] = timespan
		body["query_datetimescope_from"] = from
		body["query_datetimescope_to"] = to
		body["query_datetimescope_column"] = query.TimeColumn
	}

	if len(query.Resources) > 1 && query.QueryType == dataquery.AzureQueryTypeAzureLogAnalytics && !query.AppInsightsQuery {
		str := strings.ToLower(query.Resources[0])

		if strings.Contains(str, "microsoft.operationalinsights/workspaces") {
			body["workspaces"] = query.Resources
		} else {
			body["resources"] = query.Resources
		}
	}

	if query.AppInsightsQuery {
		body["applications"] = query.Resources
	}

	jsonValue, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", "failed to create request", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queryURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, fmt.Errorf("%v: %w", "failed to create request", err)
	}

	req.URL.Path = "/"
	req.Header.Set("Content-Type", "application/json")
	req.URL.Path = path.Join(req.URL.Path, query.URL)

	return req, nil
}

type AzureLogAnalyticsURLResources struct {
	Resources []AzureLogAnalyticsURLResource `json:"resources"`
}

type AzureLogAnalyticsURLResource struct {
	ResourceID string `json:"resourceId"`
}

func getQueryUrl(query string, resources []string, azurePortalUrl string, timeRange backend.TimeRange) (string, error) {
	encodedQuery, err := encodeQuery(query)
	if err != nil {
		return "", fmt.Errorf("failed to encode the query: %s", err)
	}

	portalUrl := azurePortalUrl + "/#blade/Microsoft_OperationsManagementSuite_Workspace/AnalyticsBlade/initiator/AnalyticsShareLinkToQuery/isQueryEditorVisible/true/scope/"
	resourcesJson := AzureLogAnalyticsURLResources{
		Resources: make([]AzureLogAnalyticsURLResource, 0),
	}
	for _, resource := range resources {
		resourcesJson.Resources = append(resourcesJson.Resources, AzureLogAnalyticsURLResource{
			ResourceID: resource,
		})
	}
	resourcesMarshalled, err := json.Marshal(resourcesJson)
	if err != nil {
		return "", fmt.Errorf("failed to marshal log analytics resources: %s", err)
	}
	from := timeRange.From.Format(time.RFC3339)
	to := timeRange.To.Format(time.RFC3339)
	timespan := url.QueryEscape(fmt.Sprintf("%s/%s", from, to))
	portalUrl += url.QueryEscape(string(resourcesMarshalled))
	portalUrl += "/query/" + url.PathEscape(encodedQuery) + "/isQueryBase64Compressed/true/timespan/" + timespan
	return portalUrl, nil
}

func getTracesQueryUrl(resources []string, azurePortalUrl string) (string, error) {
	portalUrl := azurePortalUrl
	portalUrl += "/#view/AppInsightsExtension/DetailsV2Blade/ComponentId~/"
	resource := struct {
		ResourceId string `json:"ResourceId"`
	}{
		resources[0],
	}
	resourceMarshalled, err := json.Marshal(resource)
	if err != nil {
		return "", fmt.Errorf("failed to marshal application insights resource: %s", err)
	}

	portalUrl += url.PathEscape(string(resourceMarshalled))
	portalUrl += "/DataModel~/"

	// We're making use of data link variables to select the necessary fields in the frontend
	eventId := "%22eventId%22%3A%22${__data.fields.itemId}%22%2C"
	timestamp := "%22timestamp%22%3A%22${__data.fields.startTime}%22%2C"
	eventTable := "%22eventTable%22%3A%22${__data.fields.itemType}%22"
	traceObject := fmt.Sprintf("%%7B%s%s%s%%7D", eventId, timestamp, eventTable)

	portalUrl += traceObject

	return portalUrl, nil
}

func getCorrelationWorkspaces(ctx context.Context, baseResource string, resourcesMap map[string]bool, dsInfo types.DatasourceInfo, operationId string) (map[string]bool, error) {
	azMonService := dsInfo.Services["Azure Monitor"]
	correlationUrl := azMonService.URL + fmt.Sprintf("%s/providers/microsoft.insights/transactions/%s", baseResource, operationId)

	callCorrelationAPI := func(url string) (AzureCorrelationAPIResponse, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer([]byte{}))
		if err != nil {
			return AzureCorrelationAPIResponse{}, fmt.Errorf("%v: %w", "failed to create request", err)
		}
		req.URL.Path = url
		req.Header.Set("Content-Type", "application/json")
		values := req.URL.Query()
		values.Add("api-version", "2019-10-17-preview")
		req.URL.RawQuery = values.Encode()
		req.Method = "GET"

		_, span := tracing.DefaultTracer().Start(ctx, "azure traces correlation request", trace.WithAttributes(
			attribute.String("target", req.URL.String()),
			attribute.Int64("datasource_id", dsInfo.DatasourceID),
			attribute.Int64("org_id", dsInfo.OrgID),
		))
		defer span.End()

		res, err := azMonService.HTTPClient.Do(req)
		if err != nil {
			return AzureCorrelationAPIResponse{}, err
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return AzureCorrelationAPIResponse{}, err
		}

		defer func() {
			if err := res.Body.Close(); err != nil {
				azMonService.Logger.Warn("Failed to close response body", "err", err)
			}
		}()

		if res.StatusCode/100 != 2 {
			return AzureCorrelationAPIResponse{}, fmt.Errorf("request failed, status: %s, body: %s", res.Status, string(body))
		}
		var data AzureCorrelationAPIResponse
		d := json.NewDecoder(bytes.NewReader(body))
		d.UseNumber()
		err = d.Decode(&data)
		if err != nil {
			return AzureCorrelationAPIResponse{}, err
		}

		for _, resource := range data.Properties.Resources {
			lowerCaseResource := strings.ToLower(resource)
			if _, ok := resourcesMap[lowerCaseResource]; !ok {
				resourcesMap[lowerCaseResource] = true
			}
		}
		return data, nil
	}

	var nextLink *string
	var correlationResponse AzureCorrelationAPIResponse

	correlationResponse, err := callCorrelationAPI(correlationUrl)
	if err != nil {
		return nil, err
	}
	nextLink = correlationResponse.Properties.NextLink

	for nextLink != nil {
		correlationResponse, err := callCorrelationAPI(correlationUrl)
		if err != nil {
			return nil, err
		}
		nextLink = correlationResponse.Properties.NextLink
	}

	// Remove the base element as that's where the query is run anyway
	delete(resourcesMap, strings.ToLower(baseResource))
	return resourcesMap, nil
}

// GetPrimaryResultTable returns the first table in the response named "PrimaryResult", or an
// error if there is no table by that name.
func (ar *AzureLogAnalyticsResponse) GetPrimaryResultTable() (*types.AzureResponseTable, error) {
	for _, t := range ar.Tables {
		if t.Name == "PrimaryResult" {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("no data as PrimaryResult table is missing from the response")
}

func (e *AzureLogAnalyticsDatasource) unmarshalResponse(res *http.Response) (AzureLogAnalyticsResponse, error) {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return AzureLogAnalyticsResponse{}, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			e.Logger.Warn("Failed to close response body", "err", err)
		}
	}()

	if res.StatusCode/100 != 2 {
		return AzureLogAnalyticsResponse{}, fmt.Errorf("request failed, status: %s, body: %s", res.Status, string(body))
	}

	var data AzureLogAnalyticsResponse
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	err = d.Decode(&data)
	if err != nil {
		return AzureLogAnalyticsResponse{}, err
	}

	return data, nil
}

// LogAnalyticsMeta is a type for the a Frame's Meta's Custom property.
type LogAnalyticsMeta struct {
	ColumnTypes     []string `json:"azureColumnTypes"`
	AzurePortalLink string   `json:"azurePortalLink,omitempty"`
}

// encodeQuery encodes the query in gzip so the frontend can build links.
func encodeQuery(rawQuery string) (string, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write([]byte(rawQuery)); err != nil {
		return "", err
	}

	if err := gz.Close(); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}
