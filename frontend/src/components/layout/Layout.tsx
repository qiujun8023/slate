// Mono Press 全局壳：masthead + 用户下拉。无 tab，只有 logo + dropdown。

import { Link, Outlet } from 'react-router-dom';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { LogOut } from 'lucide-react';
import { useAuth } from '@/features/auth/hooks/useAuth';
import { IconBlock } from '@/components/ui/IconBlock';
import { appRoutes } from '@/app/routes';

export function Layout() {
  const { user, logout } = useAuth();

  return (
    <div className="min-h-screen flex flex-col bg-paper">
      <header className="border-b border-ink bg-paper">
        <div className="max-w-6xl mx-auto px-5 sm:px-8 py-3.5 flex items-center justify-between gap-4">
          {/* logo */}
          <Link to={appRoutes.home} className="flex items-center gap-3 min-w-0">
            <IconBlock size="md" tone="brand" className="font-serif text-[18px] font-bold">
              墨
            </IconBlock>
            <div className="min-w-0">
              <p className="font-serif text-[18px] font-bold leading-none text-ink tracking-tight truncate">
                Slate
              </p>
              <p className="font-sans text-[10px] text-stone leading-none mt-1 tracking-[0.2em] uppercase truncate">
                案头那块墨水屏
              </p>
            </div>
          </Link>

          {/* 右侧用户下拉 */}
          <div className="flex items-center gap-2">
            {user && (
              <DropdownMenu.Root>
                <DropdownMenu.Trigger asChild>
                  <button
                    aria-label="账号菜单"
                    className="group inline-flex items-center gap-2 h-9 pl-1.5 pr-3 text-[13px] text-stone border border-ink hover:bg-cream-deep transition-colors"
                  >
                    <span className="w-6 h-6 bg-ink text-paper flex items-center justify-center font-sans text-[11px] font-semibold">
                      {(user.username ?? user.email).charAt(0).toUpperCase()}
                    </span>
                    <span className="hidden md:inline font-sans max-w-[140px] truncate text-ink">
                      {user.username ?? user.email}
                    </span>
                    <span className="font-mono text-[10px] text-stone">▾</span>
                  </button>
                </DropdownMenu.Trigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.Content
                    align="end"
                    sideOffset={6}
                    className="min-w-[200px] bg-paper border border-ink shadow-dropdown py-1 z-50"
                  >
                    <div className="px-3 pt-2 pb-2.5 border-b border-line mb-1">
                      <p className="text-[10px] text-stone uppercase tracking-[0.16em] font-mono">
                        已登录
                      </p>
                      <p className="text-[13px] text-ink truncate font-sans mt-0.5">{user.email}</p>
                    </div>
                    <DropdownMenu.Item
                      onSelect={() => logout()}
                      className="flex items-center gap-3 mx-1 px-3 py-2 text-[13px] text-clay cursor-pointer hover:bg-cream outline-none"
                    >
                      <LogOut size={14} />
                      退出登录
                    </DropdownMenu.Item>
                  </DropdownMenu.Content>
                </DropdownMenu.Portal>
              </DropdownMenu.Root>
            )}
          </div>
        </div>
      </header>

      <main className="flex-1 max-w-6xl w-full mx-auto px-5 sm:px-8 pt-7 sm:pt-9 pb-16">
        <Outlet />
      </main>

      <footer className="border-t border-line">
        <div className="max-w-6xl mx-auto px-5 sm:px-8 py-3.5 text-center">
          <p className="font-mono text-[11px] text-stone tracking-[0.06em]">
            Slate · 1bpp · 400×300 · v0.1
          </p>
        </div>
      </footer>
    </div>
  );
}
