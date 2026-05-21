package typescript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"costEngine/internal/entity/node"
)

// decoder carrega o config necessário para materializar URNs/Meta a
// partir dos payloads NDJSON. O sidecar não repete `repo`/`runID`/
// `observedAt` em cada evento — Go-side aplica.
type decoder struct {
	cfg            Config
	res            Result
	callerOrd      map[node.URN]int
	seenFrameworks map[node.URN]bool // dedup Framework URNs (declared + inferred via Calls)
}

// decodeStream lê NDJSON do reader até `done` ou `error`, agregando
// entidades por kind. Errors são devolvidos cedo; chamador é
// responsável por matar o processo.
//
// Eventos desconhecidos são ignorados (forward-compat): sidecars
// futuros podem emitir novos kinds, e o Go-side antigo só descarta.
func decodeStream(r io.Reader, cfg Config) (Result, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	d := &decoder{cfg: cfg}

	var (
		gotInit    bool
		gotDone    bool
		errPayload *ErrorPayload
	)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var env Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return d.res, fmt.Errorf("decode envelope: %w (line=%q)", err, truncate(line, 200))
		}
		if env.Schema != "" && env.Schema != SchemaVersion {
			return d.res, fmt.Errorf("%w: sidecar=%s expected=%s", ErrSidecarSchema, env.Schema, SchemaVersion)
		}

		switch env.Kind {
		case EventInit:
			gotInit = true
		case EventService:
			var p ServicePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode service: %w", err)
			}
			d.appendService(p)
		case EventModule:
			var p ModulePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode module: %w", err)
			}
			d.appendModule(p)
		case EventEndpoint:
			var p EndpointPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode endpoint: %w", err)
			}
			d.appendEndpoint(p)
		case EventFunction:
			var p FunctionPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode function: %w", err)
			}
			d.appendFunction(p)
		case EventCall:
			var p CallPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode call: %w", err)
			}
			d.appendCall(p)
		case EventType:
			var p TypePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode type: %w", err)
			}
			d.appendType(p)
		case EventVariable:
			var p VariablePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode variable: %w", err)
			}
			d.appendVariable(p)
		case EventFramework:
			var p FrameworkPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode framework: %w", err)
			}
			d.appendFramework(p)
		case EventEdge:
			var p EdgePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode edge: %w", err)
			}
			d.appendEdge(p)
		case EventProgress:
			// MVP: descarta. Slice futura conecta a logger.
		case EventError:
			var p ErrorPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return d.res, fmt.Errorf("decode error event: %w", err)
			}
			errPayload = &p
			gotDone = true
		case EventDone:
			gotDone = true
		default:
			// Forward-compat: kinds desconhecidos são ignorados.
		}

		if gotDone {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return d.res, fmt.Errorf("read stream: %w", err)
	}
	if errPayload != nil {
		return d.res, fmt.Errorf("sidecar reported error [%s]: %s (where=%s)",
			errPayload.Code, errPayload.Message, errPayload.Where)
	}
	if !gotInit {
		return d.res, fmt.Errorf("sidecar finished without init event")
	}
	if !gotDone {
		return d.res, fmt.Errorf("sidecar finished without done event")
	}
	// Pós-pass: resolve Endpoint->Function (Invokes) e Call->Function (Targets)
	// via heurística de last-segment. Sidecar não faz symbol-resolution.
	d.resolveTargets()
	return d.res, nil
}

func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	out := make([]byte, n+3)
	copy(out, b[:n])
	copy(out[n:], "...")
	return out
}
