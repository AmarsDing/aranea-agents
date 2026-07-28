package cmd

import (
	"fmt"

	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewKnowledgeCmd creates the `aranea knowledge` command group.
func NewKnowledgeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "knowledge",
		Short: "知识库管理（集合 / 文档 / 检索）",
	}
	c.AddCommand(
		knowledgeCollectionsCmd(),
		knowledgeDocumentsCmd(),
		knowledgeSearchCmd(),
	)
	return c
}

func knowledgeCollectionsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "collections",
		Short: "知识库集合管理",
	}
	c.AddCommand(
		knowledgeCollectionsLsCmd(),
		knowledgeCollectionsGetCmd(),
		knowledgeCollectionsCreateCmd(),
		knowledgeCollectionsDeleteCmd(),
	)
	return c
}

func knowledgeCollectionsLsCmd() *cobra.Command {
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出知识库集合",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListCollections(cmd.Context(), limit, offset)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(knowledgeCollectionsToRows(resp.Items), int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func knowledgeCollectionsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看知识库集合详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			col, err := cc.Client.GetCollection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(knowledgeCollectionToRow(col))
		},
	}
}

func knowledgeCollectionsCreateCmd() *cobra.Command {
	var name, description, embeddingModel string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建知识库集合",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &knowledgev1.CreateCollectionRequest{
				Name:           name,
				Description:    description,
				EmbeddingModel: embeddingModel,
			}
			col, err := cc.Client.CreateCollection(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("知识库集合创建成功", "id", col.Id, "name", col.Name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "集合名称")
	cmd.Flags().StringVar(&description, "description", "", "集合描述")
	cmd.Flags().StringVar(&embeddingModel, "embedding-model", "", "Embedding 模型")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("embedding-model")
	return cmd
}

func knowledgeCollectionsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除知识库集合（需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认删除知识库集合 %q？集合内文档与向量将一并删除，此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteCollection(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("知识库集合已删除", "id", args[0])
		},
	}
}

func knowledgeDocumentsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "documents",
		Short: "知识库文档管理",
	}
	c.AddCommand(
		knowledgeDocumentsLsCmd(),
		knowledgeDocumentsGetCmd(),
		knowledgeDocumentsDeleteCmd(),
	)
	return c
}

func knowledgeDocumentsLsCmd() *cobra.Command {
	var collectionID string
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出知识库文档",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListDocuments(cmd.Context(), collectionID, limit, offset)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(knowledgeDocumentsToRows(resp.Items), int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&collectionID, "collection-id", "", "按集合 ID 过滤")
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func knowledgeDocumentsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看文档内容（提取/组织后的文本）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			doc, err := cc.Client.GetDocumentContent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(documentContentToRow(doc))
		},
	}
}

func knowledgeDocumentsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除知识库文档（需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认删除知识库文档 %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteDocument(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("知识库文档已删除", "id", args[0])
		},
	}
}

func knowledgeSearchCmd() *cobra.Command {
	var collectionID, query string
	var topK int32
	cmd := &cobra.Command{
		Use:   "search",
		Short: "向量检索知识库",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &knowledgev1.SearchRequest{
				CollectionId: collectionID,
				Query:        query,
				TopK:         topK,
			}
			resp, err := cc.Client.SearchKnowledge(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(knowledgeChunksToRows(resp.Chunks), len(resp.Chunks))
		},
	}
	cmd.Flags().StringVar(&collectionID, "collection-id", "", "集合 ID（空 = 跨集合联邦检索）")
	cmd.Flags().StringVar(&query, "query", "", "检索内容")
	cmd.Flags().Int32Var(&topK, "top-k", 5, "返回条数")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

// Row helpers convert proto items to display rows.

func knowledgeCollectionToRow(c *knowledgev1.KnowledgeCollection) map[string]string {
	if c == nil {
		return nil
	}
	return map[string]string{
		"id":              c.Id,
		"name":            c.Name,
		"status":          c.Status,
		"embedding_model": c.EmbeddingModel,
		"document_count":  fmt.Sprintf("%d", c.DocumentCount),
		"chunk_count":     fmt.Sprintf("%d", c.ChunkCount),
		"created_at":      c.CreatedAt,
	}
}

func knowledgeCollectionsToRows(items []*knowledgev1.KnowledgeCollection) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, c := range items {
		rows = append(rows, knowledgeCollectionToRow(c))
	}
	return rows
}

func knowledgeDocumentsToRows(items []*knowledgev1.KnowledgeDocument) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, d := range items {
		rows = append(rows, map[string]string{
			"id":            d.Id,
			"collection_id": d.CollectionId,
			"source":        d.Source,
			"status":        d.Status,
			"chunk_count":   fmt.Sprintf("%d", d.ChunkCount),
			"created_at":    d.CreatedAt,
		})
	}
	return rows
}

func documentContentToRow(d *knowledgev1.DocumentContent) map[string]string {
	if d == nil {
		return nil
	}
	organized := "false"
	if d.Organized {
		organized = "true"
	}
	return map[string]string{
		"id":        d.Id,
		"organized": organized,
		"content":   d.ContentText,
	}
}

func knowledgeChunksToRows(items []*knowledgev1.KnowledgeChunk) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, ch := range items {
		rows = append(rows, map[string]string{
			"id":          ch.Id,
			"doc_id":      ch.DocId,
			"score":       fmt.Sprintf("%.3f", ch.Score),
			"chunk_index": fmt.Sprintf("%d", ch.ChunkIndex),
			"content":     ch.Content,
		})
	}
	return rows
}
