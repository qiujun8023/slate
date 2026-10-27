#pragma once

// 内容组为空或未选组时的居中提示,FrameScene 和 BgRefreshScene 共用。

#include <lvgl.h>

#include "ui/theme.h"

inline lv_obj_t* CreateEmptyGroupLabel(lv_obj_t* parent) {
    auto* label = lv_label_create(parent);
    lv_obj_set_style_text_font(label, &Zfull_16, 0);
    lv_obj_set_style_text_color(label, lv_color_black(), 0);
    lv_obj_set_style_text_align(label, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_style_text_line_space(label, 8, 0);
    lv_label_set_long_mode(label, LV_LABEL_LONG_WRAP);
    lv_obj_set_width(label, LV_HOR_RES - 32);
    lv_label_set_text(label, "内容组暂无内容\n\n请在管理端添加内容");
    lv_obj_align(label, LV_ALIGN_CENTER, 0, 0);
    return label;
}
