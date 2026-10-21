"""Native, editable PowerPoint entrance effects; no generated code."""
from pptx.oxml import parse_xml
from pptx.oxml.ns import nsdecls


def add_click_sequence(slide, shapes):
    effects = []
    for index, shape in enumerate(shapes):
        node = 3 + index * 4
        effects.append(f'''<p:par><p:cTn id="{node}" fill="hold">
          <p:stCondLst><p:cond delay="indefinite"/></p:stCondLst><p:childTnLst>
            <p:par><p:cTn id="{node+1}" fill="hold"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst>
              <p:par><p:cTn id="{node+2}" presetID="1" presetClass="entr" presetSubtype="0" fill="hold" nodeType="clickEffect">
                <p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst><p:set><p:cBhvr>
                  <p:cTn id="{node+3}" dur="1" fill="hold"><p:stCondLst><p:cond delay="0"/></p:stCondLst></p:cTn>
                  <p:tgtEl><p:spTgt spid="{shape.shape_id}"/></p:tgtEl>
                  <p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst>
                </p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst>
              </p:cTn></p:par>
            </p:childTnLst></p:cTn></p:par>
          </p:childTnLst></p:cTn></p:par>''')
    timing = parse_xml(f'''<p:timing {nsdecls('p')}><p:tnLst>
      <p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>
        <p:seq concurrent="1" nextAc="seek"><p:cTn id="2" dur="indefinite" nodeType="mainSeq"><p:childTnLst>
          {''.join(effects)}
        </p:childTnLst></p:cTn>
        <p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst>
        <p:nextCondLst><p:cond evt="onNext" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:nextCondLst>
        </p:seq>
      </p:childTnLst></p:cTn></p:par>
    </p:tnLst></p:timing>''')
    slide._element.insert_element_before(timing, 'p:extLst')
