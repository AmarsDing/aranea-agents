package main

import (
  "fmt"
  jsoncodec "github.com/go-kratos/kratos/v2/encoding/json"
  v1 "aranea-agents/api/kratos/llm_provider_model/v1"
)

type UpdateProviderModelRequest struct {
  Id            string
  ProviderModel *v1.ProviderModel
}

func main() {
  var in UpdateProviderModelRequest
  data := []byte(`{"configJson":"{\"input_price_micro_usd_per_1k\":12}","key":"k","name":"n"}`)
  err := jsoncodec.CodecForRequest(nil, "") 
  _ = err
  err = (jsoncodec.Codec{}).Unmarshal(data, &in.ProviderModel)
  fmt.Printf("err=%v in.ProviderModel=%v cfg=%q\n", err, in.ProviderModel != nil, "")
  if in.ProviderModel != nil {
    fmt.Printf("configJson=%q\n", in.ProviderModel.ConfigJson)
  }
}
