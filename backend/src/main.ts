import 'reflect-metadata';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { Logger as NestLogger, RequestMethod } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { FastifyAdapter, type NestFastifyApplication } from '@nestjs/platform-fastify';
import { Logger as PinoLogger } from 'nestjs-pino';
import { AppModule } from './app.module';
import { AppConfig } from './infra/config/app.config';
import { isStaticAssetPath } from './infra/assets/static-assets';

const MODULE_DIR = import.meta.dirname ?? import.meta.dir;
const bootstrapLogger = new NestLogger('Bootstrap');

async function bootstrap(): Promise<void> {
  const app = await NestFactory.create<NestFastifyApplication>(
    AppModule,
    // 只信任本机和内网的反代，公网直连的客户端不能用 X-Forwarded-For 伪造 IP 绕过限流
    new FastifyAdapter({
      trustProxy: ['loopback', 'linklocal', 'uniquelocal'],
      bodyLimit: 32 * 1024 * 1024,
    }),
    { bufferLogs: true }
  );
  app.useLogger(app.get(PinoLogger));

  const config = app.get(AppConfig);

  const cookiePlugin = (await import('@fastify/cookie')).default;
  const multipartPlugin = (await import('@fastify/multipart')).default;
  await registerFastifyPlugin(app, cookiePlugin);
  await registerFastifyPlugin(app, multipartPlugin, {
    limits: { fileSize: 32 * 1024 * 1024 },
  });
  // 注：@fastify/rate-limit 装了但不在这里 register —— Nest+Fastify 适配层不暴露
  // route-level Fastify config，全局 rate-limit 不便单独保护 ingest 端点。
  // 改在 common/rate-limit/rate-limit-guard.ts 实现按路由元数据维度限速。

  app.setGlobalPrefix('api/v1', {
    exclude: [{ path: 'healthz', method: RequestMethod.GET }],
  });

  // 单镜像生产部署 serve frontend dist；dev 模式 dist 不存在则跳过（走 vite dev server）。
  // backend/src/main.ts → ../../frontend/dist，runtime 镜像里同位置（/app/backend/src）。
  const distRoot = join(MODULE_DIR, '..', '..', 'frontend', 'dist');
  if (existsSync(distRoot)) {
    app.useStaticAssets({ root: distRoot, wildcard: false });

    // SPA fallback：NestFastifyApp init 阶段已经自己 setNotFoundHandler 一次，
    // fastify 同 prefix 不允许重复注册 → 改用 onSend hook 在响应发送前拦 404 改写。
    // 启动时一次性读 index.html 缓存，免每次磁盘 IO。
    // 带扩展名的资源（/favicon.ico 等）保持 404，避免 <img> 拿到 HTML 报损坏。
    const indexHtml = readFileSync(join(distRoot, 'index.html'));
    const fastify = app.getHttpAdapter().getInstance();
    fastify.addHook('onSend', async (req, reply, payload) => {
      if (reply.statusCode !== 404) return payload;
      const path = (req.url.split('?')[0] ?? '') as string;
      if (path.startsWith('/api/') || path === '/healthz') return payload;
      if (isStaticAssetPath(path)) return payload;
      void reply.code(200).type('text/html; charset=utf-8');
      return indexHtml;
    });
  }

  app.enableShutdownHooks();
  await app.listen({ port: config.port, host: '0.0.0.0' });
}

bootstrap().catch((err: unknown) => {
  bootstrapLogger.error(
    'Backend bootstrap failed.',
    err instanceof Error ? err.stack : String(err)
  );
  process.exitCode = 1;
});

interface FastifyPluginRegistrar {
  register(plugin: unknown, opts?: unknown): Promise<unknown>;
}

function registerFastifyPlugin(
  app: NestFastifyApplication,
  plugin: unknown,
  opts?: unknown
): Promise<unknown> {
  return (app as FastifyPluginRegistrar).register(plugin, opts);
}
