import { Project, SourceFile, CallExpression, SyntaxKind, FunctionDeclaration, MethodDeclaration, Node } from "ts-morph";
import * as path from "path";
import { emit } from "../emit";
import { CallPayload } from "../proto";
import { Service } from "./services";
import { isWithinService } from "./functions";
import { moduleNamespaceForFile } from "./modules";

/**
 * Detecta call sites dentro de cada Function/Method e classifica como:
 *   - http! : axios.* / fetch / got / HttpService.*
 *   - db!   : prisma.<model>.* / TypeORM repo.* / mongoose.<model>.* /
 *             sequelize.query / knex.*
 *   - mq!   : amqplib channel.publish / sqs.sendMessage / kafkajs producer.send
 *   - in-process : ref a Function declarada no mesmo Service (best-effort)
 *   - unresolved! : qualquer outra coisa
 *
 * Edges emitidas:
 *   - Invokes Function→Call (sempre)  ← Go-side (precisa da Call URN)
 *   - Uses Call→Framework (quando reconhecido)  ← Go-side (precisa da Call URN);
 *     sidecar só propaga `framework_name` no CallPayload.
 *   - Targets Call→Function (quando in-process resolvido)
 */
export function collectCalls(
  project: Project,
  repoRoot: string,
  repoURNBase: string,
  service: Service,
  functionURNs: Map<string, string>,
): number {
  let count = 0;

  for (const sf of project.getSourceFiles()) {
    if (!isWithinService(sf, service)) continue;
    const fileRel = path.relative(repoRoot, sf.getFilePath()).split(path.sep).join("/");
    const moduleNs = moduleNamespaceForFile(repoRoot, service, fileRel);

    // Para cada Function/Method, varrer suas Call sites.
    const callers: { name: string; node: FunctionDeclaration | MethodDeclaration }[] = [];
    for (const fn of sf.getFunctions()) {
      const name = fn.getName?.();
      if (name) callers.push({ name, node: fn });
    }
    for (const cls of sf.getClasses()) {
      const clsName = cls.getName() || "(anonymous)";
      for (const m of cls.getMethods()) {
        callers.push({ name: `(${clsName}).${m.getName()}`, node: m });
      }
    }

    for (const caller of callers) {
      const fnURN = functionURNs.get(`${fileRel}:${caller.name}`);
      if (!fnURN) continue;

      caller.node.forEachDescendant((node) => {
        if (node.getKind() !== SyntaxKind.CallExpression) return;
        const call = node as CallExpression;
        const expr = call.getExpression();
        const calleeText = expr.getText();
        const classified = classify(calleeText);
        const { line } = sf.getLineAndColumnAtPos(call.getStart());

        const payload: CallPayload = {
          service_module_path: service.modulePath,
          from_function_urn: fnURN,
          subkind: classified.subkind,
          callee_expression: calleeText,
          target_hint: classified.hint,
          framework_name: classified.frameworkName,
          location: { file: fileRel, line },
        };
        emit("call", payload);
        count++;
      });
    }
  }
  return count;
}

interface Classified {
  subkind: "http!" | "db!" | "mq!" | "in-process" | "unresolved!";
  hint?: string;
  frameworkName?: string;
}

function classify(calleeText: string): Classified {
  // HTTP
  if (/^axios(\.(get|post|put|delete|patch|head|options|request))?$/.test(calleeText)) {
    return { subkind: "http!", frameworkName: "axios" };
  }
  if (calleeText === "fetch" || /^globalThis\.fetch$/.test(calleeText)) {
    return { subkind: "http!", frameworkName: "fetch" };
  }
  if (/^got(\.(get|post|put|delete|patch|head|options))?$/.test(calleeText)) {
    return { subkind: "http!", frameworkName: "got" };
  }
  if (/HttpService\.(get|post|put|delete|patch|head|options)$/.test(calleeText)) {
    return { subkind: "http!", frameworkName: "@nestjs/axios" };
  }

  // DB
  if (/^prisma\.[a-zA-Z_]+\.(findUnique|findFirst|findMany|create|update|delete|upsert|count|aggregate|groupBy)$/.test(calleeText)) {
    const m = calleeText.match(/^prisma\.([a-zA-Z_]+)\./);
    return { subkind: "db!", frameworkName: "@prisma/client", hint: m?.[1] };
  }
  if (/Repository\.(find|findOne|findOneBy|save|update|delete|insert|count|create|remove)$/.test(calleeText)) {
    return { subkind: "db!", frameworkName: "typeorm" };
  }
  if (/Model\.(find|findOne|findById|create|updateOne|deleteOne|save|aggregate|count)$/.test(calleeText)) {
    return { subkind: "db!", frameworkName: "mongoose" };
  }
  if (/^knex\(/.test(calleeText) || /^knex\.[a-zA-Z]/.test(calleeText)) {
    return { subkind: "db!", frameworkName: "knex" };
  }

  // MQ
  if (/channel\.publish$/.test(calleeText) || /channel\.sendToQueue$/.test(calleeText)) {
    return { subkind: "mq!", frameworkName: "amqplib" };
  }
  if (/sqs\.sendMessage$/i.test(calleeText)) {
    return { subkind: "mq!", frameworkName: "@aws-sdk/client-sqs" };
  }
  if (/producer\.send$/.test(calleeText)) {
    return { subkind: "mq!", frameworkName: "kafkajs" };
  }
  if (/sns\.publish$/i.test(calleeText)) {
    return { subkind: "mq!", frameworkName: "@aws-sdk/client-sns" };
  }

  return { subkind: "unresolved!" };
}
