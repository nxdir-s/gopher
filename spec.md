## Description

`gopher` is a CLI tool that helps generate Go code, templates, and project scaffolding

#### Example Usage

```bash
gopher generate {cli_option} {cli_option}
```

## Why?

Code generated using AI usually doesn't follow my personal standards for writing Go. Things like code architecture, variable naming, error handling, code spacing, etc are always off and having to go back manually to fix it is time consuming. I want to create a CLI tool that can be used by Claude Code to create Go files out of premade templates that follow my style. Claude Code will call the tool and pass in a type to generate (Adapter, Web Server, Module, etc). The types will be common types of files or even whole project scaffoldings like an http server. The CLI will require Claude Code / Users to pass in flags that will fill the templates. Once the files are generated, Claude Code can read them and make further adjustments and edits based on the context.

## Types

The different types used for generation

- setup
- adapter
- webserver / api
- port
- valobj
- entity
- domain
- service
- module
- infra / cdk
- test
- mocks

## Configuration

Templates used by the CLI should be configurable by users. There should be a required structure to the templates but the specifics/styles should be editable.

## Templates

### Adapters

##### KafkaAdapter

```go
package adapters

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	MaxPollFetches int = 1000
)

type ErrProtoMarshal struct {
	err error
}

func (e *ErrProtoMarshal) Error() string {
	return "failed to marshal protobuf: " + e.err.Error()
}

type ErrCreateTopic struct {
	err error
}

func (e *ErrCreateTopic) Error() string {
	return "failed to create kafka topic: " + e.err.Error()
}

type ErrListTopics struct {
	err error
}

func (e *ErrListTopics) Error() string {
	return "failed to list kafka topics: " + e.err.Error()
}

type KafkaOpt func(a *KafkaAdapter) error

func WithConsumer(topic string, groupname string, brokers []string, opts ...kgo.Opt) KafkaOpt {
	return func(a *KafkaAdapter) error {
		options := []kgo.Opt{
			kgo.SeedBrokers(brokers...),
			kgo.ConsumerGroup(groupname),
			kgo.ConsumeTopics(topic),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
			kgo.DisableAutoCommit(),
			kgo.BlockRebalanceOnPoll(),
		}

		options = append(options, opts...)

		client, err := kgo.NewClient(options...)
		if err != nil {
			return err
		}

		a.topic = topic
		a.client = client
		a.groupName = groupname

		return nil
	}
}

func WithProducer(topic string, brokers []string, opts ...kgo.Opt) KafkaOpt {
	return func(a *KafkaAdapter) error {
		options := []kgo.Opt{
			kgo.SeedBrokers(brokers...),
		}

		options = append(options, opts...)

		client, err := kgo.NewClient(options...)
		if err != nil {
			return err
		}

		a.topic = topic
		a.client = client

		return nil
	}
}

func WithKafkaSpanAttrs(attrs ...attribute.KeyValue) KafkaOpt {
	return func(a *KafkaAdapter) error {
		a.attributes = append(a.attributes, attrs...)
		return nil
	}
}

type KafkaConsumer interface {
	Process(ctx context.Context, record *kgo.Record) error
}

type KafkaAdapter struct {
	client     *kgo.Client
	logger     *slog.Logger
	tracer     trace.Tracer
	attributes []attribute.KeyValue
	topic      string
	groupName  string
}

// NewKafkaAdapter creates a new kafka adapter. KafkaAdapters should be configured to either produce or consume, but not both
func NewKafkaAdapter(logger *slog.Logger, tracer trace.Tracer, opts ...KafkaOpt) (*KafkaAdapter, error) {
	adapter := &KafkaAdapter{
		logger: logger,
		tracer: tracer,
	}

	for _, opt := range opts {
		if err := opt(adapter); err != nil {
			return nil, err
		}
	}

	return adapter, nil
}

// Send sends the supplied kafka record to the configured topic
func (a *KafkaAdapter) Send(ctx context.Context, record protoreflect.ProtoMessage) error {
	if a.client == nil {
		a.logger.Error("nil client in KafkaAdapter")
		return nil
	}

	ctx, span := a.tracer.Start(ctx, "send "+a.topic,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", a.topic),
			attribute.String("messaging.operation.name", "send"),
			attribute.String("messaging.operation.type", "send"),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	data, err := proto.Marshal(record)
	if err != nil {
		a.logger.Error("error encoding record",
			slog.String("err", err.Error()),
			slog.String("topic", a.topic),
		)

		return &ErrProtoMarshal{err}
	}

	a.logger.Debug("sending kafka record")

	a.client.Produce(ctx, &kgo.Record{Topic: a.topic, Value: data}, func(_ *kgo.Record, err error) {
		if err != nil {
			a.logger.Error("record had a produce error", slog.String("err", err.Error()))
		}
	})

	return nil
}

// Consume uses the supplied consumer to process kafka records from the configured topic
func (a *KafkaAdapter) Consume(ctx context.Context, consumer KafkaConsumer) {
	if a.client == nil {
		a.logger.Error("nil client in KafkaAdapter")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			ctx, span := a.tracer.Start(ctx, "receive "+a.topic,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					attribute.String("messaging.system", "kafka"),
					attribute.String("messaging.consumer.group.name", a.groupName),
					attribute.String("messaging.operation.name", "receive"),
					attribute.String("messaging.operation.type", "receive"),
				),
			)

			if a.attributes != nil {
				span.SetAttributes(a.attributes...)
			}

			fetches := a.client.PollRecords(ctx, MaxPollFetches)
			span.End()

			ctx, span = a.tracer.Start(ctx, "process "+a.topic,
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("messaging.system", "kafka"),
					attribute.String("messaging.consumer.group.name", a.groupName),
					attribute.String("messaging.operation.name", "process"),
					attribute.String("messaging.operation.type", "process"),
				),
			)

			if a.attributes != nil {
				span.SetAttributes(a.attributes...)
			}

			if errors := fetches.Errors(); len(errors) > 0 {
				for _, e := range errors {
					if e.Err == context.Canceled {
						a.logger.Error("received interrupt", slog.String("err", e.Err.Error()))
						span.End()

						return
					}

					a.logger.Error("poll error", slog.String("err", e.Err.Error()))
				}
			}

			iter := fetches.RecordIter()
			for !iter.Done() {
				record := iter.Next()

				if err := consumer.Process(ctx, record); err != nil {
					a.logger.Error("error processing "+a.topic+" record", slog.String("err", err.Error()))
					span.End()

					return
				}

				a.logger.Info("consumed record", slog.String("topic", a.topic))
			}

			if err := a.client.CommitUncommittedOffsets(ctx); err != nil {
				if err == context.Canceled {
					a.logger.Error("received interrupt", slog.String("err", err.Error()))
					span.End()

					return
				}

				a.logger.Error("unable to commit offsets", slog.String("err", err.Error()))
			}

			a.client.AllowRebalance()
			span.End()
		}
	}
}

// Close closes the kafka client
func (a *KafkaAdapter) Close() error {
	if a.client == nil {
		return nil
	}

	a.client.Close()

	return nil
}

// CreateTopic creates a kafka topic
func (a *KafkaAdapter) CreateTopic(ctx context.Context, topic string) error {
	if a.client == nil {
		a.logger.Error("nil client in KafkaAdapter")
		return nil
	}

	ctx, span := a.tracer.Start(ctx, "create topic "+a.topic,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", topic),
			attribute.String("messaging.operation.name", "create topic"),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	adminClient := kadm.NewClient(a.client)

	topicDetails, err := adminClient.ListTopics(ctx)
	if err != nil {
		return &ErrListTopics{err}
	}

	if topicDetails.Has(topic) {
		a.logger.Warn("kafka topic already exists", slog.String("topic", topic))
		return nil
	}

	a.logger.Info("creating kafka topic", slog.String("topic", topic))

	if _, err := adminClient.CreateTopic(ctx, 1, -1, nil, topic); err != nil {
		a.logger.Error("failed to create kafka topic",
			slog.String("err", err.Error()),
			slog.String("topic", topic),
		)

		return &ErrCreateTopic{err}
	}

	return nil
}
```

##### PostgresAdapter

```go
package adapters

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ErrPgxConn struct {
	err error
}

func (e *ErrPgxConn) Error() string {
	return "error creating connection pool: " + e.err.Error()
}

type ErrPgInsert struct {
	err error
}

func (e *ErrPgInsert) Error() string {
	return "failed to insert row: " + e.err.Error()
}

type ErrPgDelete struct {
	err error
}

func (e *ErrPgDelete) Error() string {
	return "failed to delete row: " + e.err.Error()
}

type ErrQueryRow struct {
	err error
}

func (e *ErrQueryRow) Error() string {
	return "error querying row: " + e.err.Error()
}

type ErrRowExists struct {
	err error
}

func (e *ErrRowExists) Error() string {
	return "failed to check if row exists: " + e.err.Error()
}

type ErrExecQuery struct {
	err error
}

func (e *ErrExecQuery) Error() string {
	return "error executing query: " + e.err.Error()
}

type ErrNoRows struct{}

func (e *ErrNoRows) Error() string {
	return "error no rows affected"
}

type ErrCollectRows struct {
	err error
}

func (e *ErrCollectRows) Error() string {
	return "failed to collect rows to struct: " + e.err.Error()
}

type ErrNilPgxTx struct{}

func (e *ErrNilPgxTx) Error() string {
	return "error nil PgxTx"
}

type ErrNilPgxPool struct{}

func (e *ErrNilPgxPool) Error() string {
	return "error nil connection pool"
}

type ErrInvalidStmt struct{}

func (e *ErrInvalidStmt) Error() string {
	return "invalid sql statement"
}

const (
	SelectKeyword string = "select"
	InsertKeyword string = "insert"
	DeleteKeyword string = "delete"
)

type PgxPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

// NewPgxPool creates a pgxpool.Pool
func NewPgxPool(ctx context.Context, connUrl string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connUrl)
	if err != nil {
		return nil, &ErrPgxConn{err}
	}

	return pool, nil
}

type PgxTx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type PostgresOpt func(a *PostgresAdapter) error

// WithPgxTx sets the transaction adapter
func WithPgxTx(tx PgxTx) PostgresOpt {
	return func(a *PostgresAdapter) error {
		a.tx = tx
		return nil
	}
}

// WithPgxPool sets the adapter's connection
func WithPgxPool(pool PgxPool) PostgresOpt {
	return func(a *PostgresAdapter) error {
		a.conn = pool
		return nil
	}
}

func WithPgSpanAttrs(attrs ...attribute.KeyValue) PostgresOpt {
	return func(a *PostgresAdapter) error {
		a.attributes = append(a.attributes, attrs...)
		return nil
	}
}

type PostgresAdapter struct {
	conn       PgxPool
	tx         PgxTx
	logger     *slog.Logger
	tracer     trace.Tracer
	attributes []attribute.KeyValue
}

type SPostgresAdapter[T any] struct {
	*PostgresAdapter
}

// NewPostgresAdapter creates a new PostgresAdapter
func NewPostgresAdapter(ctx context.Context, logger *slog.Logger, tracer trace.Tracer, opts ...PostgresOpt) *PostgresAdapter {
	adapter := &PostgresAdapter{
		logger: logger,
		tracer: tracer,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// NewTransactionAdapter creates an adapter for executing transactions
func (a *PostgresAdapter) NewTransactionAdapter(ctx context.Context) (*PostgresAdapter, error) {
	if a.conn == nil {
		a.logger.Error("nil connection in PostgresAdapter")
		return nil, &ErrNilPgxPool{}
	}

	tx, err := a.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	txAdapter := NewPostgresAdapter(ctx, a.logger, a.tracer,
		WithPgxPool(tx),
		WithPgxTx(tx),
	)

	return txAdapter, nil
}

// Commit commits the transaction after first checking if the context has been canceled
func (a *PostgresAdapter) Commit(ctx context.Context) error {
	if a.tx == nil {
		a.logger.Error("nil PgxTx in PostgresAdapter")
		return &ErrNilPgxTx{}
	}

	select {
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()

		return a.tx.Rollback(ctx)
	default:
		return a.tx.Commit(ctx)
	}
}

// Rollback initiates a transaction rollback
func (a *PostgresAdapter) Rollback(ctx context.Context) error {
	if a.tx == nil {
		a.logger.Error("nil PgxTx in PostgresAdapter")
		return &ErrNilPgxTx{}
	}

	return a.tx.Rollback(ctx)
}

// ConnectionPool returns the underlying pgxpool
func (a *PostgresAdapter) ConnectionPool() PgxPool {
	return a.conn
}

// Exec executes the supplied sql statement and returns the number of rows affected
func (a *PostgresAdapter) Exec(ctx context.Context, sql string, args map[string]interface{}) (int64, error) {
	if a.conn == nil {
		a.logger.Error("nil connection in PostgresAdapter")
		return 0, &ErrNilPgxPool{}
	}

	ctx, span := a.tracer.Start(ctx, "EXEC",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.query.text", sql),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	resp, err := a.conn.Exec(ctx, sql, args)
	if err != nil {
		return 0, &ErrExecQuery{err}
	}

	return resp.RowsAffected(), nil
}

// Insert creates a new row returns the id
func (a *PostgresAdapter) Insert(ctx context.Context, sql string, args map[string]interface{}) (int, error) {
	if a.conn == nil {
		a.logger.Error("nil connection in PostgresAdapter")
		return 0, &ErrNilPgxPool{}
	}

	if !validStatement(InsertKeyword, sql) {
		return 0, &ErrInvalidStmt{}
	}

	ctx, span := a.tracer.Start(ctx, "INSERT",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "INSERT"),
			attribute.String("db.query.text", sql),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	var id int
	if err := a.conn.QueryRow(ctx, sql, args).Scan(&id); err != nil {
		return 0, &ErrPgInsert{err}
	}

	return id, nil
}

const ExInsertQuery string = `
    INSERT INTO table (
        data
    ) VALUES (
        @data
    ) RETURNING id
`

// Delete deletes the supplied row
func (a *PostgresAdapter) Delete(ctx context.Context, sql string, args map[string]interface{}) error {
	if a.conn == nil {
		a.logger.Error("nil connection in PostgresAdapter")
		return &ErrNilPgxPool{}
	}

	if !validStatement(DeleteKeyword, sql) {
		return &ErrInvalidStmt{}
	}

	ctx, span := a.tracer.Start(ctx, "DELETE",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "DELETE"),
			attribute.String("db.query.text", sql),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	resp, err := a.conn.Exec(ctx, sql, args)
	if err != nil {
		return &ErrPgDelete{err}
	}

	if resp.RowsAffected() == 0 {
		return &ErrNoRows{}
	}

	return nil
}

const ExDeleteQuery string = `
    DELETE FROM table
    WHERE id = @id
`

func (a *PostgresAdapter) RowExists(ctx context.Context, sql string, args map[string]interface{}) (bool, error) {
	if a.conn == nil {
		a.logger.Error("nil connection in PostgresAdapter")
		return false, &ErrNilPgxPool{}
	}

	if !validStatement(SelectKeyword, sql) {
		return false, &ErrInvalidStmt{}
	}

	ctx, span := a.tracer.Start(ctx, "SELECT",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
			attribute.String("db.query.text", sql),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	var id int
	err := a.conn.QueryRow(ctx, sql, args).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, &ErrRowExists{err}
	}

	if id == 0 || errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	return true, nil
}

const ExRowExistsQuery string = `
    SELECT id
    FROM table
    WHERE id = @id
`

func validStatement(keyword string, sql string) bool {
	stmt := strings.TrimLeftFunc(sql, unicode.IsSpace)

	return strings.ToLower(keyword) == strings.ToLower(stmt[:len(keyword)])
}
```

##### HttpAdapter

```go
package adapters

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nxdir-s/grl/internal/core/entity"
	"github.com/nxdir-s/grl/internal/core/valobj"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	_ int64 = 1 << (10 * iota)
	Kib
	Mib
	Gib
)

const (
	DefaultTimeout         int   = 10
	DefaultMaxIdleConns    int   = 100
	DefaultMaxConnsPerHost int   = 100
	DefaultReadByteLimit   int64 = 15 * Mib
	DefaultRetryLimit      int   = 3

	ContentTypeHeader         string = "Content-Type"
	ContentTypeFormURLEncoded string = "application/x-www-form-urlencoded"
)

type ErrHttpCopy struct {
	err error
}

func (e *ErrHttpCopy) Error() string {
	return "failed to copy request body: " + e.err.Error()
}

type ErrHttpStatusCode struct {
	code int
	msg  *bytes.Buffer
}

func (e *ErrHttpStatusCode) Error() string {
	return "recieved bad status code: " + e.msg.String()
}

type ErrReadRespBody struct {
	err error
}

func (e *ErrReadRespBody) Error() string {
	return "failed to read response body: " + e.err.Error()
}

type ErrNilRequest struct{}

func (e *ErrNilRequest) Error() string {
	return "recieved nil request"
}

type ErrAttachFile struct {
	path string
	err  error
}

func (e *ErrAttachFile) Error() string {
	return "failed to attach '" + e.path + "': " + e.err.Error()
}

type HttpOpt func(a *HttpAdapter)

func WithHttpClient(client *http.Client) HttpOpt {
	return func(a *HttpAdapter) {
		a.http = client
	}
}

func WithCredentials(ctx context.Context, id string, secret string, authUrl string, scopes ...string) HttpOpt {
	return func(a *HttpAdapter) {
		credentials := &clientcredentials.Config{
			ClientID:     id,
			ClientSecret: secret,
			TokenURL:     authUrl,
			Scopes:       make([]string, 0, len(scopes)),
		}

		credentials.Scopes = append(credentials.Scopes, scopes...)

		a.http = credentials.Client(context.WithValue(ctx, oauth2.HTTPClient, a.http))
	}
}

type HttpConfig struct {
	TlsConfig             *tls.Config
	RetryEnabled          bool
	FollowRedirects       bool
	Timeout               int
	RetryLimit            int
	MaxIdleConnections    int
	MaxConnectionsPerHost int
	ReadByteLimit         int64
}

type HttpAdapter struct {
	http   *http.Client
	limit  int64
	logger *slog.Logger
}

func NewHttpAdapter(cfg *HttpConfig, logger *slog.Logger, opts ...HttpOpt) *HttpAdapter {
	var timeout int = DefaultTimeout * int(time.Second)
	if cfg.Timeout != 0 {
		timeout = cfg.Timeout
	}

	var byteLimit int64 = DefaultReadByteLimit
	if cfg.ReadByteLimit != 0 {
		byteLimit = cfg.ReadByteLimit
	}

	var maxIdleConns int = DefaultMaxIdleConns
	if cfg.MaxIdleConnections != 0 {
		maxIdleConns = cfg.MaxIdleConnections
	}

	var maxConnsPerHost int = DefaultMaxConnsPerHost
	if cfg.MaxConnectionsPerHost != 0 {
		maxConnsPerHost = cfg.MaxConnectionsPerHost
	}

	defaultTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: time.Duration(timeout),
		}).Dial,
		TLSClientConfig:     cfg.TlsConfig,
		MaxIdleConns:        maxIdleConns,
		MaxConnsPerHost:     maxConnsPerHost,
		MaxIdleConnsPerHost: maxConnsPerHost,
		IdleConnTimeout:     time.Duration(timeout),
		TLSHandshakeTimeout: time.Duration(timeout),
	}

	var transport http.RoundTripper = defaultTransport

	if cfg.RetryEnabled {
		transport = NewRetryTransport(defaultTransport, cfg.RetryLimit)
	}

	adapter := &HttpAdapter{
		http: &http.Client{
			Timeout:   time.Duration(timeout),
			Transport: transport,
		},
		limit:  byteLimit,
		logger: logger,
	}

	if !cfg.FollowRedirects {
		adapter.http.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

func (a *HttpAdapter) Send(ctx context.Context, req *entity.Request) (*entity.Response, error) {
	if req == nil {
		return nil, &ErrNilRequest{}
	}

	body, err := a.encodeBody(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method.String(), req.URL, body)
	if err != nil {
		return nil, err
	}

	a.applyHeaders(httpReq, req)

	var timing valobj.Timing
	var dnsStart, connectStart, tlsStart, gotConn time.Time
	start := time.Now()

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			timing.DNSLookup = time.Since(dnsStart)
		},
		ConnectStart: func(_, _ string) {
			connectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			timing.TCPConnect = time.Since(connectStart)
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			timing.TLSHandshake = time.Since(tlsStart)
		},
		GotConn: func(_ httptrace.GotConnInfo) {
			gotConn = time.Now()
		},
		GotFirstResponseByte: func() {
			if !gotConn.IsZero() {
				timing.TTFB = time.Since(gotConn)
			}
		},
	}

	httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), trace))

	httpResp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	timing.Total = time.Since(start)

	respBody := &bytes.Buffer{}
	if _, err := io.Copy(respBody, io.LimitReader(httpResp.Body, a.limit)); err != nil {
		return nil, &ErrReadRespBody{err}
	}

	headers := make([]valobj.Header, 0, len(httpResp.Header))
	for key, values := range httpResp.Header {
		for i := range values {
			headers = append(headers, valobj.Header{
				Key:     key,
				Value:   values[i],
				Enabled: true,
			})
		}
	}

	return &entity.Response{
		StatusCode:    httpResp.StatusCode,
		Status:        httpResp.Status,
		Headers:       headers,
		Body:          respBody,
		ContentType:   httpResp.Header.Get(ContentTypeHeader),
		ContentLength: httpResp.ContentLength,
		Timing:        timing,
	}, nil
}

func (a *HttpAdapter) encodeBody(req *entity.Request) (io.Reader, error) {
	switch req.BodyType {
	case valobj.BodyTypeFormURL:
		v := url.Values{}

		for i := range req.FormFields {
			if !req.FormFields[i].Enabled || len(req.FormFields[i].Key) == 0 {
				continue
			}

			v.Add(req.FormFields[i].Key, req.FormFields[i].Value)
		}

		req.ContentType = ContentTypeFormURLEncoded

		return strings.NewReader(v.Encode()), nil
	case valobj.BodyTypeFormData:
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)

		for i := range req.FormFields {
			if !req.FormFields[i].Enabled || len(req.FormFields[i].Key) == 0 {
				continue
			}

			if strings.HasPrefix(req.FormFields[i].Value, "@") {
				path := strings.TrimPrefix(req.FormFields[i].Value, "@")

				file, err := os.Open(path)
				if err != nil {
					return nil, &ErrAttachFile{path, err}
				}

				part, err := w.CreateFormFile(req.FormFields[i].Key, filepath.Base(path))
				if err != nil {
					file.Close()
					return nil, err
				}

				if _, err := io.Copy(part, file); err != nil {
					file.Close()
					return nil, err
				}

				file.Close()
				continue
			}

			if err := w.WriteField(req.FormFields[i].Key, req.FormFields[i].Value); err != nil {
				return nil, err
			}
		}

		if err := w.Close(); err != nil {
			return nil, err
		}

		req.ContentType = w.FormDataContentType()

		return buf, nil
	default:
		if len(req.Body) == 0 {
			return nil, nil
		}

		return strings.NewReader(req.Body), nil
	}
}

func (a *HttpAdapter) applyHeaders(httpReq *http.Request, req *entity.Request) {
	for i := range req.Headers {
		if !req.Headers[i].Enabled || len(req.Headers[i].Key) == 0 {
			continue
		}

		httpReq.Header.Set(req.Headers[i].Key, req.Headers[i].Value)
	}

	if len(req.ContentType) != 0 {
		httpReq.Header.Set(ContentTypeHeader, req.ContentType)
	}
}

type RetryTransport struct {
	transport http.RoundTripper
	retryMax  int
}

// NewRetryTransport wraps the supplied http transport with a retryable implementation
func NewRetryTransport(transport *http.Transport, limit int) *RetryTransport {
	var retryLimit int = DefaultRetryLimit
	if limit != 0 {
		retryLimit = limit
	}

	return &RetryTransport{
		transport: transport,
		retryMax:  retryLimit,
	}
}

// RoundTrip implements the http.RoundTripper interface with retries
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	var err error

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, &ErrHttpCopy{err}
		}

		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	resp, err := t.transport.RoundTrip(req)

	retries := 0
	for shouldRetry(resp, err) && retries < t.retryMax {
		time.Sleep(backoff(retries))

		if resp.Body != nil {
			drainBody(resp.Body)
		}

		if req.Body != nil {
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		resp, err = t.transport.RoundTrip(req)

		retries++
	}

	return resp, err
}

func drainBody(body io.ReadCloser) error {
	defer body.Close()

	if _, err := io.ReadAll(body); err != nil {
		return err
	}

	return nil
}

// shouldRetry checks for errors and non 2XX status codes to determine whether to retry
func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}

	if resp.StatusCode/10 != 20 {
		return true
	}

	return false
}

// backoff doubles the delay
func backoff(retries int) time.Duration {
	return time.Duration(math.Pow(2, float64(retries))) * time.Second
}
```

##### AWSAdapter

```go
package adapters

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/smithy-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	MaxDeleteKeys int = 1000
)

type ErrNilAWSClient struct {
	client string
}

func (e *ErrNilAWSClient) Error() string {
	return "missing client: " + e.client
}

type ErrTypeCast struct {
	value string
}

func (e *ErrTypeCast) Error() string {
	return "failed to type cast " + e.value
}

type ErrGetObject struct {
	err error
}

func (e *ErrGetObject) Error() string {
	return "failed to get object from s3: " + e.err.Error()
}

type ErrPutObject struct {
	err error
}

func (e *ErrPutObject) Error() string {
	return "failed to upload to s3: " + e.err.Error()
}

type ErrHeadObject struct {
	err error
}

func (e *ErrHeadObject) Error() string {
	return "failed HeadObject request to s3: " + e.err.Error()
}

type ErrListObjects struct {
	err error
}

func (e *ErrListObjects) Error() string {
	return "failed to list s3 objects: " + e.err.Error()
}

type ErrDeleteObjects struct {
	err error
}

func (e *ErrDeleteObjects) Error() string {
	return "failed to delete s3 objects: " + e.err.Error()
}

type ErrS3Output struct {
	api string
}

func (e *ErrS3Output) Error() string {
	return "found one or more errors in " + e.api + " output"
}

type ErrGetSecret struct {
	err error
}

func (e *ErrGetSecret) Error() string {
	return "failed to get secret: " + e.err.Error()
}

type AWSOpt func(a *AWSAdapter)

func WithS3Client(client S3Client) AWSOpt {
	return func(a *AWSAdapter) {
		a.s3 = client
	}
}

func WithSecretsManagerClient(client SecretsManagerClient) AWSOpt {
	return func(a *AWSAdapter) {
		a.sm = client
	}
}

func WithAWSSpanAttrs(attrs ...attribute.KeyValue) AWSOpt {
	return func(a *AWSAdapter) {
		a.attributes = append(a.attributes, attrs...)
	}
}

type S3Client interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

type ListObjectsV2Pager interface {
	HasMorePages() bool
	NextPage(ctx context.Context, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type ObjectNotExistsWaiter interface {
	Wait(ctx context.Context, params *s3.HeadObjectInput, maxWaitDur time.Duration, optFns ...func(*s3.ObjectNotExistsWaiterOptions)) error
}

type SecretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

type AWSAdapter struct {
	s3         S3Client
	sm         SecretsManagerClient
	logger     *slog.Logger
	tracer     trace.Tracer
	attributes []attribute.KeyValue
}

func NewAWSAdapter(logger *slog.Logger, tracer trace.Tracer, opts ...AWSOpt) *AWSAdapter {
	adapter := &AWSAdapter{
		logger: logger,
		tracer: tracer,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

func (a *AWSAdapter) GetObject(ctx context.Context, bucket string, key string) (io.Reader, error) {
	if a.s3 == nil {
		return nil, &ErrNilAWSClient{"s3"}
	}

	ctx, span := a.tracer.Start(ctx, "S3 GetObject",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.key", key),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	a.logger.Info("reading object from s3",
		slog.String("bucket", bucket),
		slog.String("key", key),
	)

	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	output, err := a.s3.GetObject(ctx, params)
	if err != nil {
		a.logger.Error("failed to read object from s3",
			slog.String("bucket", bucket),
			slog.String("key", key),
			slog.String("err", err.Error()),
		)

		return nil, &ErrGetObject{err}
	}

	pr, pw := io.Pipe()

	go func() {
		if _, err := io.Copy(pw, output.Body); err != nil {
			a.logger.Error("failed to copy GetObject output",
				slog.String("bucket", bucket),
				slog.String("key", key),
				slog.String("err", err.Error()),
			)

			return
		}
	}()

	return pr, nil
}

func (a *AWSAdapter) PutObject(ctx context.Context, data io.Reader, bucket string, key string) error {
	if a.s3 == nil {
		return &ErrNilAWSClient{"s3"}
	}

	ctx, span := a.tracer.Start(ctx, "S3 PutObject",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.key", key),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	a.logger.Info("uploading to s3",
		slog.String("bucket", bucket),
		slog.String("key", key),
	)

	params := &s3.PutObjectInput{
		Bucket:               aws.String(bucket),
		Key:                  aws.String(key),
		Body:                 data,
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	}

	if _, err := a.s3.PutObject(ctx, params); err != nil {
		a.logger.Error("failed to upload object to s3",
			slog.String("bucket", bucket),
			slog.String("key", key),
			slog.String("err", err.Error()),
		)

		return &ErrPutObject{err}

	}

	return nil
}

const (
	S3ObjectNotExists int = iota
	S3ObjectExists
)

func (a *AWSAdapter) ObjectExists(ctx context.Context, bucket string, key string) (int, error) {
	if a.s3 == nil {
		return S3ObjectNotExists, &ErrNilAWSClient{"s3"}
	}

	ctx, span := a.tracer.Start(ctx, "S3 ObjectExists",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("aws.s3.bucket", bucket),
			attribute.String("aws.s3.key", key),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	a.logger.Info("checking if object exists in s3",
		slog.String("bucket", bucket),
		slog.String("key", key),
	)

	params := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	if _, err := a.s3.HeadObject(ctx, params); err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) && apiError.ErrorCode() == "NotFound" {
			a.logger.Info("object does not exist in s3",
				slog.String("bucket", bucket),
				slog.String("key", key),
			)

			return S3ObjectNotExists, nil
		}

		return S3ObjectNotExists, &ErrHeadObject{err}
	}

	a.logger.Info("object exists in s3",
		slog.String("bucket", bucket),
		slog.String("key", key),
	)

	return S3ObjectExists, nil
}

func (a *AWSAdapter) ListObjects(ctx context.Context, bucket string, prefix string, pager interface{}) ([]string, error) {
	if a.s3 == nil {
		return nil, &ErrNilAWSClient{"s3"}
	}

	ctx, span := a.tracer.Start(ctx, "S3 ListObjects",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("aws.s3.bucket", bucket),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	a.logger.Info("listing objects in s3",
		slog.String("bucket", bucket),
		slog.String("prefix", prefix),
	)

	objects := make([]string, 0)
	params := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	if len(prefix) > 0 {
		params.Prefix = &prefix
		span.SetAttributes(attribute.String("aws.s3.prefix", prefix))
	}

	var objectPaginator ListObjectsV2Pager
	objectPaginator = s3.NewListObjectsV2Paginator(a.s3, params)

	if pager != nil {
		v2Pager, ok := pager.(ListObjectsV2Pager)
		if !ok {
			return nil, &ErrTypeCast{"ListObjectsV2Pager"}
		}

		objectPaginator = v2Pager
	}

	for objectPaginator.HasMorePages() {
		output, err := objectPaginator.NextPage(ctx)
		if err != nil {
			a.logger.Error("failed to list s3 objects",
				slog.String("bucket", bucket),
				slog.String("prefix", prefix),
				slog.String("err", err.Error()),
			)

			return nil, &ErrListObjects{err}
		}

		for i := range output.Contents {
			objects = append(objects, *output.Contents[i].Key)
		}
	}

	a.logger.Info("objects found",
		slog.String("bucket", bucket),
		slog.String("prefix", prefix),
		slog.Int("count", len(objects)),
	)

	return objects, nil
}

func (a *AWSAdapter) DeleteObjects(ctx context.Context, bucket string, objects []string, waiter interface{}) error {
	if a.s3 == nil {
		return &ErrNilAWSClient{"s3"}
	}

	if len(objects) == 0 {
		return nil
	}

	ctx, span := a.tracer.Start(ctx, "S3 DeleteObjects",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("aws.s3.bucket", bucket),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	a.logger.Info("deleting objects from s3",
		slog.String("bucket", bucket),
		slog.Int("count", len(objects)),
	)

	objectIds := make([]types.ObjectIdentifier, 0, len(objects))
	for i := range objects {
		objectIds = append(objectIds, types.ObjectIdentifier{
			Key: aws.String(objects[i]),
		})
	}

	deleteIndex := 0
	objectsExist := true

	for objectsExist {
		objectKeys := getDeleteList(objectIds, deleteIndex)

		params := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{
				Objects: objectKeys,
				Quiet:   aws.Bool(true),
			},
		}

		output, err := a.s3.DeleteObjects(ctx, params)
		if err != nil {
			return &ErrDeleteObjects{err}
		}

		if len(output.Errors) > 0 {
			for i := range output.Errors {
				a.logger.Error("failed to delete s3 object",
					slog.String("bucket", bucket),
					slog.String("key", *output.Errors[i].Key),
					slog.String("err", *output.Errors[i].Message),
				)
			}

			return &ErrDeleteObjects{
				&ErrS3Output{"DeleteObjects"},
			}
		}

		for i := range output.Deleted {
			params := &s3.HeadObjectInput{
				Bucket: aws.String(bucket),
				Key:    output.Deleted[i].Key,
			}

			var s3Waiter ObjectNotExistsWaiter
			s3Waiter = s3.NewObjectNotExistsWaiter(a.s3)

			if waiter != nil {
				objectWaiter, ok := waiter.(ObjectNotExistsWaiter)
				if !ok {
					return &ErrTypeCast{"ObjectNotExistsWaiter"}
				}

				s3Waiter = objectWaiter
			}

			if err := s3Waiter.Wait(ctx, params, time.Minute); err != nil {
				a.logger.Warn("timed out waiting for object deletion",
					slog.String("bucket", bucket),
					slog.String("key", *output.Deleted[i].Key),
				)

				continue
			}

			a.logger.Info("object deleted",
				slog.String("bucket", bucket),
				slog.String("key", *output.Deleted[i].Key),
			)
		}

		deleteIndex += len(objectKeys)
		if deleteIndex >= len(objectKeys) {
			objectsExist = false
		}
	}

	return nil
}

func getDeleteList(objectIds []types.ObjectIdentifier, index int) []types.ObjectIdentifier {
	if len(objectIds) > MaxDeleteKeys {
		if index+MaxDeleteKeys > len(objectIds) {
			return objectIds[index:]
		}

		return objectIds[index : index+MaxDeleteKeys]
	}

	return objectIds
}

func (a *AWSAdapter) GetSecret(ctx context.Context, arn string, name string) (string, error) {
	if a.sm == nil {
		return "", &ErrNilAWSClient{"secretsmanager"}
	}

	ctx, span := a.tracer.Start(ctx, "SecretsManager GetSecretValue",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("aws.secretsmanager.secretName", name),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	params := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(arn),
	}

	a.logger.Info("retrieving secret from secrets manager",
		slog.String("secretName", name),
	)

	output, err := a.sm.GetSecretValue(ctx, params)
	if err != nil {
		return "", &ErrGetSecret{err}
	}

	return *output.SecretString, nil
}
```

##### GoogleAdapter

```go
package adapters

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type ErrNilConfig struct{}

func (e *ErrNilConfig) Error() string {
	return "error nil config"
}

type ErrNilToken struct{}

func (e *ErrNilToken) Error() string {
	return "error nil token"
}

type ErrNilClient struct{}

func (e *ErrNilClient) Error() string {
	return "error nil client"
}

type ErrNilDrive struct{}

func (e *ErrNilDrive) Error() string {
	return "error nil drive"
}

type ErrAuthCode struct {
	err error
}

func (e *ErrAuthCode) Error() string {
	return "unable to read authorization code: " + e.err.Error()
}

type ErrWebToken struct {
	err error
}

func (e *ErrWebToken) Error() string {
	return "unable to retrieve token from web: " + e.err.Error()
}

type ErrDriveClient struct {
	err error
}

func (e *ErrDriveClient) Error() string {
	return "unable to retrieve drive client: " + e.err.Error()
}

type ErrOpenFile struct {
	err    error
	source string
}

func (e *ErrOpenFile) Error() string {
	return "error opening " + e.source + " : " + e.err.Error()
}

type ErrDriveUpload struct {
	err error
}

func (e *ErrDriveUpload) Error() string {
	return "error uploading to drive: " + e.err.Error()
}

type GoogleOpt func(a *GoogleAdapter) error

func WithCredentialsJSON(ctx context.Context, source string) GoogleOpt {
	return func(a *GoogleAdapter) error {
		credentialsJSON, err := os.ReadFile(source)
		if err != nil {
			return err
		}

		config, err := google.ConfigFromJSON(credentialsJSON)
		if err != nil {
			return err
		}

		a.config = config

		fmt.Printf("Go to the following link in your browser then type the authorization code: \n%v\n", a.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline))

		var authCode string
		if _, err := fmt.Scan(&authCode); err != nil {
			return &ErrAuthCode{err}
		}

		token, err := a.config.Exchange(ctx, authCode)
		if err != nil {
			return &ErrWebToken{err}
		}

		a.token = token
		a.client = a.config.Client(ctx, a.token)

		return nil
	}
}

func WithDriveConn(ctx context.Context) GoogleOpt {
	return func(a *GoogleAdapter) error {
		if a.client == nil {
			return &ErrNilClient{}
		}

		client, err := drive.NewService(ctx, option.WithHTTPClient(a.client))
		if err != nil {
			return &ErrDriveClient{err}
		}

		a.drive = client

		return nil
	}
}

func WithGoogleSpanAttrs(attrs ...attribute.KeyValue) GoogleOpt {
	return func(a *GoogleAdapter) error {
		a.attributes = append(a.attributes, attrs...)
		return nil
	}
}

type GoogleAdapter struct {
	config     *oauth2.Config
	token      *oauth2.Token
	client     *http.Client
	drive      *drive.Service
	tracer     trace.Tracer
	attributes []attribute.KeyValue
}

func NewGoogleAdapter(tracer trace.Tracer, opts ...GoogleOpt) (*GoogleAdapter, error) {
	adapter := &GoogleAdapter{
		tracer: tracer,
	}

	for _, opt := range opts {
		if err := opt(adapter); err != nil {
			return nil, err
		}
	}

	return adapter, nil
}

func (a *GoogleAdapter) Upload(ctx context.Context, file io.Reader, name string) error {
	if a.drive == nil {
		return &ErrNilDrive{}
	}

	_, span := a.tracer.Start(ctx, "Google Upload",
		trace.WithLinks(trace.LinkFromContext(ctx)),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("file.name", name),
		),
	)
	defer span.End()

	if a.attributes != nil {
		span.SetAttributes(a.attributes...)
	}

	df := &drive.File{
		Name: name,
	}

	if _, err := a.drive.Files.Create(df).Media(file).Do(); err != nil {
		return &ErrDriveUpload{err}
	}

	return nil
}
```

##### CmdAdapter

```go
package adapters

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

type CmdAdapter struct{}

func NewCmdAdapter() *CmdAdapter {
	return &CmdAdapter{}
}

func (a *CmdAdapter) Exec(ctx context.Context, cmd *exec.Cmd) (io.Reader, error) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(output), nil
}
```

##### TmuxAdapter

```go
package adapters

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type ErrHasSession struct {
	err error
}

func (e *ErrHasSession) Error() string {
	return "error checking if session exists: " + e.err.Error()
}

type ErrNewSession struct {
	session string
	err     error
}

func (e *ErrNewSession) Error() string {
	return "error creating new session named '" + e.session + "': " + e.err.Error()
}

type ErrAttachSession struct {
	session string
	err     error
}

func (e *ErrAttachSession) Error() string {
	return "error attaching to session '" + e.session + "': " + e.err.Error()
}

type ErrNewWindow struct {
	window string
	err    error
}

func (e *ErrNewWindow) Error() string {
	return "error creating new window named '" + e.window + "': " + e.err.Error()
}

type ErrSelectWindow struct {
	window string
	err    error
}

func (e *ErrSelectWindow) Error() string {
	return "error selecting " + e.window + " window: " + e.err.Error()
}

type ErrSendKeys struct {
	cmd string
	err error
}

func (e *ErrSendKeys) Error() string {
	return "error executing " + TmuxSendKeysCmd + " with cmd '" + e.cmd + "': " + e.err.Error()
}

type ErrNilCmd struct{}

func (e *ErrNilCmd) Error() string {
	return "error nil CmdAdapter"
}

const Alias string = "tmux"

const (
	TmuxSessionExists int = iota
	TmuxSessionNotExists
)

const (
	TmuxEnterCmd        string = "C-m"
	TmuxHasSessionCmd   string = "has-session"
	TmuxNewSessionCmd   string = "new-session"
	TmuxNewWindowCmd    string = "new-window"
	TmuxSelectWindowCmd string = "select-window"
	TmuxAttachCmd       string = "attach-session"
	TmuxSendKeysCmd     string = "send-keys"
)

type Command interface {
	Exec(context.Context, *exec.Cmd) (io.Reader, error)
}

type TmuxOpt func(a *TmuxAdapter)

func WithCmdAdapter(cmd Command) TmuxOpt {
	return func(a *TmuxAdapter) {
		a.cmd = cmd
	}
}

type TmuxAdapter struct {
	cmd Command
}

// NewTmuxAdapter creates a tmux adapter
func NewTmuxAdapter(ctx context.Context, opts ...TmuxOpt) *TmuxAdapter {
	adapter := &TmuxAdapter{}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// HasSession checks for an already existing tmux session
func (a *TmuxAdapter) HasSession(ctx context.Context, session string) (int, error) {
	if a.cmd == nil {
		return TmuxSessionNotExists, &ErrNilCmd{}
	}

	cmd := exec.CommandContext(ctx, Alias, TmuxHasSessionCmd, "-t", session)

	fmt.Fprintf(os.Stdout, "checking for existing session '%s'\n", session)

	output, err := a.cmd.Exec(ctx, cmd)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s failed: %s\n", TmuxHasSessionCmd, err.Error())

		return TmuxSessionNotExists, &ErrHasSession{err}
	}

	go func() {
		if _, err := io.Copy(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stdout, "error copying '%s' output to Stdout: %s\n", TmuxHasSessionCmd, err.Error())
			return
		}
	}()

	return TmuxSessionExists, nil
}

// NewSession creates a new tmux session
func (a *TmuxAdapter) NewSession(ctx context.Context, name string) error {
	if a.cmd == nil {
		return &ErrNilCmd{}
	}

	cmd := exec.CommandContext(ctx, Alias, TmuxNewSessionCmd, "-d", "-s", name, "-n", name)

	fmt.Fprintf(os.Stdout, "creating new session named '%s'\n", name)

	output, err := a.cmd.Exec(ctx, cmd)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s failed: %s\n", TmuxNewSessionCmd, err.Error())

		return &ErrNewSession{name, err}
	}

	go func() {
		if _, err := io.Copy(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stdout, "error copying '%s' output to Stdout: %s\n", TmuxNewSessionCmd, err.Error())
			return
		}
	}()

	return nil
}

// AttachSession attempts attaching to a tmux session
func (a *TmuxAdapter) AttachSession(ctx context.Context, session string) error {
	if a.cmd == nil {
		return &ErrNilCmd{}
	}

	cmd := exec.CommandContext(ctx, Alias, TmuxAttachCmd, "-t", session)
	cmd.Stdin = os.Stdin

	output, err := a.cmd.Exec(ctx, cmd)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s failed: %s\n", TmuxAttachCmd, err.Error())

		return &ErrAttachSession{session, err}
	}

	go func() {
		if _, err := io.Copy(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stdout, "error copying '%s' output to Stdout: %s\n", TmuxAttachCmd, err.Error())
			return
		}
	}()

	return nil
}

// SendKeys executes the supplied command
func (a *TmuxAdapter) SendKeys(ctx context.Context, cmd []string, session string, window string) error {
	if a.cmd == nil {
		return &ErrNilCmd{}
	}

	cmdArgs := []string{TmuxSendKeysCmd, "-t", session + ":" + window}
	cmdArgs = append(cmdArgs, cmd...)

	command := exec.CommandContext(ctx, Alias, cmdArgs...)

	output, err := a.cmd.Exec(ctx, command)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s failed: %s\n", TmuxSendKeysCmd, err.Error())

		return &ErrSendKeys{strings.Join(cmdArgs, " "), err}
	}

	go func() {
		if _, err := io.Copy(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stdout, "error copying '%s' output to Stdout: %s\n", TmuxSendKeysCmd, err.Error())
			return
		}
	}()

	return nil
}

// NewWindow creates a new tmux window
func (a *TmuxAdapter) NewWindow(ctx context.Context, session string, name string) error {
	if a.cmd == nil {
		return &ErrNilCmd{}
	}

	cmd := exec.CommandContext(ctx, Alias, TmuxNewWindowCmd, "-t", session, "-n", name)

	output, err := a.cmd.Exec(ctx, cmd)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s failed: %s\n", TmuxNewWindowCmd, err.Error())

		return &ErrNewWindow{name, err}
	}

	go func() {
		if _, err := io.Copy(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stdout, "error copying '%s' output to Stdout: %s\n", TmuxNewWindowCmd, err.Error())
			return
		}
	}()

	return nil
}

// SelectWindow selects a tmux window
func (a *TmuxAdapter) SelectWindow(ctx context.Context, session string, window string) error {
	if a.cmd == nil {
		return &ErrNilCmd{}
	}

	cmd := exec.CommandContext(ctx, Alias, TmuxSelectWindowCmd, "-t", session+":"+window)

	output, err := a.cmd.Exec(ctx, cmd)
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s failed: %s\n", TmuxSelectWindowCmd, err.Error())

		return &ErrSelectWindow{window, err}
	}

	go func() {
		if _, err := io.Copy(os.Stdout, output); err != nil {
			fmt.Fprintf(os.Stdout, "error copying '%s' output to Stdout: %s\n", TmuxSelectWindowCmd, err.Error())
			return
		}
	}()

	return nil
}
```

##### TomlAdapter

```go
package adapters

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

type ErrReadCfg struct {
	name string
	err  error
}

func (e *ErrReadCfg) Error() string {
	return "failed to read " + e.name + ": " + e.err.Error()
}

type ErrUnmarshalToml struct {
	name string
	err  error
}

func (e *ErrUnmarshalToml) Error() string {
	return "failed to unmarshal " + e.name + ": " + e.err.Error()
}

type TomlAdapter[T any] struct {
	Config T
}

// NewTomlAdapter creates a toml adapter
func NewTomlAdapter[T any]() *TomlAdapter[T] {
	return &TomlAdapter[T]{}
}

// LoadConfig attempts to read a toml file in the current directory and returns a config
func (a *TomlAdapter[T]) LoadConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return &ErrReadCfg{filename, err}
	}

	if err := toml.Unmarshal(data, &a.Config); err != nil {
		return &ErrUnmarshalToml{filename, err}
	}

	return nil
}
```

##### ZipAdapter

```go
package adapters

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ErrInvalidPath struct {
	path string
}

func (e *ErrInvalidPath) Error() string {
	return "invalid file path: " + e.path
}

type ZipAdapter struct{}

func NewZipAdapter() *ZipAdapter {
	return &ZipAdapter{}
}

func (a *ZipAdapter) Zip(ctx context.Context, src string, filename string, dst io.Writer) error {
	writer := zip.NewWriter(dst)
	defer writer.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Method = zip.Deflate

		if header.Name, err = filepath.Rel(filepath.Dir(src), path); err != nil {
			return err
		}

		if info.IsDir() {
			header.Name += "/"
		}

		headerWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(headerWriter, f); err != nil {
			return err
		}

		return nil
	})
}

func (a *ZipAdapter) Unzip(ctx context.Context, src string, dst string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	destination, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	for _, f := range reader.File {
		if err := a.unzipFile(f, destination); err != nil {
			return err
		}
	}

	return nil
}

func (a *ZipAdapter) unzipFile(f *zip.File, dst string) error {
	filePath := filepath.Join(dst, f.Name)

	if !strings.HasPrefix(filePath, filepath.Clean(dst)+string(os.PathSeparator)) {
		return &ErrInvalidPath{filePath}
	}

	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
			return err
		}

		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	zippedFile, err := f.Open()
	if err != nil {
		return err
	}
	defer zippedFile.Close()

	if _, err := io.Copy(dstFile, zippedFile); err != nil {
		return err
	}

	return nil
}
```

**Examples**

- JSONAdapter
- AppleAdapter
- APIAdapter
- TUIAdapter

**Adapters Repo:** https://github.com/nxdir-s/adapters

#### Modules

**Examples**

- auth
- logs
- config
- messages
- websockets
- server
- observability
- tests

## Hexagonal Project Structure

The idea of Hexagonal Architecture is to put inputs and outputs at the edges of our design. Business logic should not depend on whether we expose a REST or a GraphQL API, and it should not depend on where we get data from — a database, a microservice API exposed via gRPC or REST, or just a simple CSV file

The following diagrams the project structure

```
.
├── cmd
└── internal
    ├── adapters
    │   ├── postgres
    │   ├── api
    │   └── aws
    ├── config
    ├── core
    │   ├── domain
    │   ├── entity
    │   └── valobj
    ├── logs
    └── ports
```

#### cmd

The cmd directory contains your applications main modules. The `main.go` file will be responsible for setting up dependencies and running the application

#### adapters

The adapters module contains the **primary** and **secondary** adapters that handle communication between external entities and the core of the application. Adapters will be responsible for any data tranformations required for communication, as well as error handling, telemetry, and logging

#### core

The core directory contains the applications business logic and will utilize a lite version of Domain-Driven Design

There are three main concepts that define the core: **Domains**, **Entities**, and **Value Objects**

- **Domains** can be thought of as "orchestrators" for domain use cases. They implement business rules and validation logic specific to a domain
- **Entities** are the domain object types
- **Value Objects** represent shared immutable data types

#### ports

The ports module contains the port definitions that define how the core will interact with internal and external entities. It is split up into the following files:

- `core.go`
  - Contains interfaces that define how domains interact with internal and external entities
- `primary.go`
  - Contains interfaces that define how the core allows external entities to interact with it. Ex. `ports.API` defines how the core will allow APIs to interact with it
- `secondary.go`
  - Contains interfaces that define how the core wants to drive/interact with external entities. These entities are usually databases or some data source, but can also be other internal applications or even a library
