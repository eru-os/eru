package eru_otel

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	stdout "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"log"
	"time"
)

func TracerInit() (*sdktrace.TracerProvider, error) {
	exporter, err := stdout.New()
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func TracerTempoInit(traceUrl string) (*sdktrace.TracerProvider, error) {
	//initProvider()
	//flush := initProvider()
	//defer flush()

	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			// Specify the service name displayed on the backend of Tracing Analysis.
			semconv.ServiceNameKey.String("eru"),
		),
	)
	if err != nil {
		log.Print(err, "failed to create resource")
	}
	res, err = resource.Merge(resource.Default(), res)
	if err != nil {
		log.Print(err, "failed to merge resource")
	}
	traceClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		//"localhost:4317"
		otlptracegrpc.WithEndpoint(traceUrl), // Replace otelAgentAddr with the endpoint obtained in the Prerequisites section.
		//otlptracegrpc.WithHeaders(headers),
		otlptracegrpc.WithDialOption(grpc.WithBlock()))
	log.Print("connecting tempo")
	exporter, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		return nil, err
	}
	log.Print("connecting tempo success")

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	//tracerProvider := sdktrace.NewTracerProvider(
	//	sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
	//	sdktrace.WithResource(res),
	//	sdktrace.WithSpanProcessor(bsp),
	//)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil

}

func initProvider() func() {
	ctx := context.Background()

	//otelAgentAddr, xtraceToken, ok := common.ObtainXTraceInfo()

	//if !ok {
	//	log.Print("Cannot init OpenTelemetry, exit")
	//	os.Exit(-1)
	//}

	//headers := map[string]string{"Authentication": xtraceToken} // Replace xtraceToken with the authentication token obtained in the Prerequisites section.

	traceClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("localhost:55680"), // Replace otelAgentAddr with the endpoint obtained in the Prerequisites section.
		//otlptracegrpc.WithHeaders(headers),
		otlptracegrpc.WithDialOption(grpc.WithBlock()))
	log.Println("start to connect to server")
	traceExp, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		log.Print(err, "Failed to create the collector trace exporter")
	}
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			// Specify the service name displayed on the backend of Tracing Analysis.
			semconv.ServiceNameKey.String("test server"),
		),
	)
	if err != nil {
		log.Print(err, "failed to create resource")
	}

	bsp := sdktrace.NewBatchSpanProcessor(traceExp)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// Set the global propagator to tracecontext. The global propagator is not specified by default.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tracerProvider)

	return func() {
		cxt, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := traceExp.Shutdown(cxt); err != nil {
			otel.Handle(err)
		}
	}
}
