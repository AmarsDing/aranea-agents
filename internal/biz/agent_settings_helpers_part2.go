package biz

func estimateTokensForFiles(files []AgentPromptFile) FileTokenEstimates {
	result := FileTokenEstimates{}
	for _, f := range files {
		tokens := len(f.Body) / 4
		if tokens == 0 && len(f.Body) > 0 {
			tokens = 1
		}
		result.FileEstimates = append(result.FileEstimates, FileTokenEstimate{
			FileID:          f.ID,
			FileName:        f.Name,
			EstimatedTokens: tokens,
		})
		result.TotalTokens += tokens
	}
	return result
}
