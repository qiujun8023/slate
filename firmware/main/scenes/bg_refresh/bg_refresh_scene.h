#pragma once

#include <atomic>
#include <memory>
#include <string>

#include "scenes/core/scene.h"
#include "ui/status_bar.h"

class EpdSsd1683;

class BgRefreshScene : public Scene {
   public:
    ~BgRefreshScene() override;

    const char* Name() const override {
        return "bg_refresh";
    }
    void      OnEnter(SceneContext& ctx) override;
    void      OnExit(SceneContext& ctx) override;
    void      OnEvent(SceneContext& ctx, const UiEvent& e) override;
    lv_obj_t* Root() override {
        return root_;
    }
    bool RequiresRoot() const override {
        return false;
    }

   private:
    enum class State {
        kWaiting,
        kRendering,
        kDone,
    };

    bool SeedPreviousFrame(SceneContext& ctx);
    bool ResolveCurrentFrame(std::string& gid, int& seq, int& content_count);
    bool RenderChangedFrame(SceneContext& ctx);
    bool RenderEmptyState(SceneContext& ctx);
    void StartWatcher(EpdSsd1683* epd);
    void StartDeadlineWatchdog();
    void Finish();

    State                              state_                  = State::kWaiting;
    bool                               previous_screen_seeded_ = false;
    bool                               redraw_pending_         = false;
    std::shared_ptr<std::atomic<bool>> done_posted_            = std::make_shared<std::atomic<bool>>(false);

    lv_obj_t*                  root_ = nullptr;
    std::unique_ptr<StatusBar> status_bar_;
};
