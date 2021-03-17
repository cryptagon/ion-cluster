#include "gst.h"

#include <gst/app/gstappsrc.h>

typedef struct SampleHandlerUserData {
  int pipelineId;
} SampleHandlerUserData;

GMainLoop *gstreamer_main_loop = NULL;
void gstreamer_start_mainloop(void) {
  gstreamer_main_loop = g_main_loop_new(NULL, FALSE);
  g_main_loop_run(gstreamer_main_loop);
}

static gboolean gstreamer_bus_call(GstBus *bus, GstMessage *msg, gpointer data) {
  GstElement *pipeline = GST_ELEMENT(data);

  switch (GST_MESSAGE_TYPE(msg)) {
  case GST_MESSAGE_EOS:
    if (!gst_element_seek (pipeline, 1.0, GST_FORMAT_TIME, GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_KEY_UNIT | GST_SEEK_FLAG_SKIP,
             GST_SEEK_TYPE_SET, 0,
             GST_SEEK_TYPE_NONE, GST_CLOCK_TIME_NONE)) {
        g_print ("EOS restart failed\n");
        exit(1);
    }
    break;

  case GST_MESSAGE_ERROR: {
    gchar *debug;
    GError *error;

    gst_message_parse_error(msg, &error, &debug);
    g_free(debug);

    g_printerr("GStreamer Error: %s\n", error->message);
    g_error_free(error);
    exit(1);
  }
  default:
    break;
  }

  return TRUE;
}

GstElement *gstreamer_create_pipeline(char *pipeline) {
  gst_init(NULL, NULL);
  GError *error = NULL;
  return gst_parse_launch(pipeline, &error);
}

void gstreamer_start_pipeline(GstElement *pipeline) {
  GstBus *bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline));
  gst_bus_add_watch(bus, gstreamer_bus_call, pipeline);
  gst_object_unref(bus);

  // gstreamer_play_pipeline(pipeline);
}

void gstreamer_stop_pipeline(GstElement *pipeline) { 
  gst_element_set_state(pipeline, GST_STATE_NULL); 
  // gst_element_get_state(pipeline, NULL, NULL, GST_CLOCK_TIME_NONE);
}

void gstreamer_play_pipeline(GstElement *pipeline) {
  gst_element_set_state(pipeline, GST_STATE_PLAYING);
  // gst_element_get_state(pipeline, NULL, NULL, GST_CLOCK_TIME_NONE);
}

void gstreamer_pause_pipeline(GstElement *pipeline) {
  gst_element_set_state(pipeline, GST_STATE_PAUSED);
  // gst_element_get_state(pipeline, NULL, NULL, GST_CLOCK_TIME_NONE);
}

void gstreamer_seek(GstElement *pipeline, int64_t seek_pos) {
    if (!gst_element_seek (pipeline, 1.0, GST_FORMAT_TIME, GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_KEY_UNIT | GST_SEEK_FLAG_SKIP,
             GST_SEEK_TYPE_SET, seek_pos * GST_SECOND,
             GST_SEEK_TYPE_NONE, GST_CLOCK_TIME_NONE)) {
        g_print ("Seek failed!\n");
    }
}

GstFlowReturn gstreamer_send_new_sample_handler(GstElement *object, gpointer user_data) {
  GstSample *sample = NULL;
  GstBuffer *buffer = NULL;
  gpointer copy = NULL;
  gsize copy_size = 0;

  g_signal_emit_by_name (object, "pull-sample", &sample);
  if (sample) {
    buffer = gst_sample_get_buffer(sample);
    if (buffer) {
      gst_buffer_extract_dup(buffer, 0, gst_buffer_get_size(buffer), &copy, &copy_size);
      goHandlePipelineBuffer(copy, copy_size, GST_BUFFER_DURATION(buffer), user_data);
    }
    gst_sample_unref (sample);
  }

  return GST_FLOW_OK;
}

void gstreamer_send_bind_appsink_track(GstElement *pipeline, char *appSinkName, char *localTrackID) {
  GstElement *appsink = gst_bin_get_by_name(GST_BIN(pipeline), appSinkName);
  g_object_set(appsink, "emit-signals", TRUE, NULL); 
  g_signal_connect(appsink, "new-sample", G_CALLBACK(gstreamer_send_new_sample_handler), localTrackID); 
}

void gstreamer_receive_push_buffer(GstElement *pipeline, void *buffer, int len, char* element_name) {
  GstElement *src = gst_bin_get_by_name(GST_BIN(pipeline), element_name);

  if (src != NULL) {
    gpointer p = g_memdup(buffer, len);
    GstBuffer *buffer = gst_buffer_new_wrapped(p, len);
    gst_app_src_push_buffer(GST_APP_SRC(src), buffer);
    gst_object_unref(src);
  }
}

GstElement* gstreamer_compositor_add_input_track(GstElement *pipeline, char *input_description, bool isVideo) {
  GstElement *input_bin = gst_parse_bin_from_description(input_description, true, NULL);
  if (!input_bin) {
    g_printerr ("Unable to create bin for input track\n");
    return NULL;
  }
  gst_bin_add_many (GST_BIN (pipeline), input_bin, NULL);
  gst_element_sync_state_with_parent(input_bin);

  if(isVideo) {
    g_print("adding input to compositor\n");
    GstElement *compositor = gst_bin_get_by_name(GST_BIN(pipeline), "vmix");
    if(!compositor) g_printerr("no video compositor found!");
    gst_element_link(input_bin, compositor);
    gstreamer_compositor_relayout_videos(compositor);

    gst_object_unref(compositor);
  }else {
    g_print("adding input to mixer\n");
    GstElement *mixer = gst_bin_get_by_name(GST_BIN(pipeline), "amix");
    if(!mixer) g_printerr("no audio mixer found!");
    gst_element_link(input_bin, mixer);
    gst_object_unref(mixer);
  }


  return input_bin;
}



#define COMPOSITOR_VIDEO_WIDTH 1920 
#define COMPOSITOR_VIDEO_HEIGHT 1080

void gstreamer_compositor_relayout_videos(GstElement *compositor) {
  int num_videos = (compositor->numsinkpads) - 1;

  int rows, cols;
  if (num_videos <= 1) {
    rows = 1, cols = 1;
  }else if (num_videos <= 4) {
    rows = 2, cols = 2; 
  }else if (num_videos <= 16) {
    rows = 4, cols = 4; 
  }


  g_print("relayout: num_videos: %d ==> %d, %d\n", num_videos, rows, cols);

  int x = 0, y = 0;
  int w = COMPOSITOR_VIDEO_WIDTH / rows;
  int h = COMPOSITOR_VIDEO_HEIGHT / cols;

  GList *elem;
  GstPad *pad;

  int i = 0;
  for(elem = compositor->sinkpads; elem; elem = elem->next) {
    // if(i == 0) continue;
    pad = elem->data;
    g_object_set (G_OBJECT(pad), "xpos", x, NULL);
    g_object_set (G_OBJECT(pad), "ypos", y, NULL);
    g_object_set (G_OBJECT(pad), "width", w, NULL);
    g_object_set (G_OBJECT(pad), "height", h, NULL);

    g_print("layout i=%d (x,y,w,h)=>(%d,%d,%d,%d)", i, x,y,w,h);
    i++;

    x += w;
    if (x >= COMPOSITOR_VIDEO_WIDTH) {
      x = 0;
      y += h;
    }
  }

}



