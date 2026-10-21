package main

import (
	"context"
	"strings"
)

func animValidatePicture(p *TeachingPicture) error {
	if p == nil {
		return nil
	}
	if strings.HasPrefix(p.DataURL, "data:image/gif") {
		return fail(400, "教学动画配图请使用PNG/JPEG；GIF用于PPT放映")
	}
	return validateTeachingMedia(Slide{Picture: p})
}
func (s *Server) validateAnimationPicture(ctx context.Context, a Actor, p *TeachingPicture) error {
	if err := animValidatePicture(p); err != nil {
		return err
	}
	if p != nil && p.AssetID != "" {
		asset, err := s.ownedMedia(ctx, a, p.AssetID)
		if err != nil {
			return err
		}
		if asset.MIME == "image/gif" {
			return fail(400, "教学动画配图请使用PNG/JPEG；GIF用于PPT放映")
		}
	}
	return nil
}
func animNeedsPicture(sc AnimScript) bool {
	for _, s := range sc.Scenes {
		if strings.Contains(s.HTML, "data-teaching-picture") {
			return true
		}
		for _, o := range s.Objects {
			if o.Type == "illustration" && o.Kind == "uploaded" {
				return true
			}
		}
	}
	return false
}
