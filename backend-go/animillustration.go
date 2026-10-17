package main

func animValidatePicture(p *TeachingPicture) error {
	if p == nil {
		return nil
	}
	return validateTeachingMedia(Slide{Picture: p})
}
func animNeedsPicture(sc AnimScript) bool {
	for _, s := range sc.Scenes {
		for _, o := range s.Objects {
			if o.Type == "illustration" && o.Kind == "uploaded" {
				return true
			}
		}
	}
	return false
}
